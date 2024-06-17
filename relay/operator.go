package relay

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	clienttypes "github.com/cosmos/ibc-go/v8/modules/core/02-client/types"
	ibcexported "github.com/cosmos/ibc-go/v8/modules/core/exported"
	lcptypes "github.com/datachainlab/lcp-go/light-clients/lcp/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"github.com/hyperledger-labs/yui-relayer/core"
)

func (pr *Prover) OperatorSign(commitment [32]byte) ([]byte, error) {
	privKey, err := pr.getOperatorPrivateKey()
	if err != nil {
		return nil, err
	}
	return secp256k1.Sign(commitment[:], privKey)
}

func (pr *Prover) GetSignOperator() (common.Address, error) {
	privKey, err := pr.getOperatorPrivateKey()
	if err != nil {
		return common.Address{}, err
	}
	pk, err := crypto.ToECDSA(privKey)
	if err != nil {
		return common.Address{}, err
	}
	pubKey := pk.Public().(*ecdsa.PublicKey)
	return common.BytesToAddress(crypto.PubkeyToAddress(*pubKey).Bytes()), nil
}

func (pr *Prover) getOperatorPrivateKey() ([]byte, error) {
	return hex.DecodeString(strings.TrimPrefix(pr.config.OperatorPrivateKey, "0x"))
}

func (pr *Prover) IsOperatorEnabled() bool {
	return len(pr.config.OperatorPrivateKey) > 0 && pr.config.OperatorsEip712Params != nil
}

func (pr *Prover) GetOperators() ([]common.Address, error) {
	var operators []common.Address
	for _, operator := range pr.config.Operators {
		addrStr := strings.TrimPrefix(operator, "0x")
		if len(addrStr) != 40 {
			return nil, fmt.Errorf("invalid operator address length %v", len(addrStr))
		}
		addr := common.HexToAddress(operator)
		operators = append(operators, addr)
	}
	return operators, nil
}

func (pr *Prover) GetOperatorsThreshold() Fraction {
	if pr.config.OperatorsThreshold.Denominator == 0 && pr.config.OperatorsThreshold.Numerator == 0 {
		return Fraction{Numerator: 1, Denominator: 1}
	}
	return pr.config.OperatorsThreshold
}

func (pr *Prover) updateOperators(counterparty core.Chain, nonce uint64, newOperators []common.Address, threshold Fraction) error {
	if nonce == 0 {
		return fmt.Errorf("invalid nonce: %v", nonce)
	}
	if threshold.Numerator == 0 || threshold.Denominator == 0 {
		return fmt.Errorf("invalid threshold: %v", threshold)
	}

	cplatestHeight, err := counterparty.LatestHeight()
	if err != nil {
		return err
	}
	counterpartyClientRes, err := counterparty.QueryClientState(core.NewQueryContext(context.TODO(), cplatestHeight))
	if err != nil {
		return err
	}
	var cs ibcexported.ClientState
	if err := pr.codec.UnpackAny(counterpartyClientRes.ClientState, &cs); err != nil {
		return fmt.Errorf("failed to unpack client state: client_state=%v %w", counterpartyClientRes.ClientState, err)
	}
	clientState, ok := cs.(*lcptypes.ClientState)
	if !ok {
		return fmt.Errorf("failed to cast client state: %T", cs)
	}
	if len(clientState.Operators) != 1 {
		return fmt.Errorf("currently only one operator is supported, but got %v", len(clientState.Operators))
	}
	opSigner, err := pr.GetSignOperator()
	if err != nil {
		return err
	}
	if !bytes.Equal(clientState.Operators[0], opSigner.Bytes()) {
		return fmt.Errorf("operator mismatch: expected 0x%x, but got 0x%x", clientState.Operators[0], opSigner)
	}
	commitment, err := pr.ComputeEIP712UpdateOperatorsHash(
		nonce,
		newOperators,
		threshold.Numerator,
		threshold.Denominator,
	)
	if err != nil {
		return err
	}
	sig, err := pr.OperatorSign(commitment)
	if err != nil {
		return err
	}
	var ops [][]byte
	for _, operator := range newOperators {
		ops = append(ops, operator.Bytes())
	}
	message := &lcptypes.UpdateOperatorsMessage{
		Nonce:                            nonce,
		NewOperators:                     ops,
		NewOperatorsThresholdNumerator:   threshold.Numerator,
		NewOperatorsThresholdDenominator: threshold.Denominator,
		Signatures:                       [][]byte{sig},
	}
	signer, err := counterparty.GetAddress()
	if err != nil {
		return err
	}
	msg, err := clienttypes.NewMsgUpdateClient(counterparty.Path().ClientID, message, signer.String())
	if err != nil {
		return err
	}
	if _, err := counterparty.SendMsgs([]sdk.Msg{msg}); err != nil {
		return err
	}
	return nil
}
