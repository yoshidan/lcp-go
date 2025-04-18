package relay

import (
	"context"
	"fmt"
	"github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/datachainlab/lcp-go/relay/elc"
	"github.com/hyperledger-labs/yui-relayer/log"
)

func updateClient(ctx context.Context, client LCPServiceClient, anyHeader *types.Any, elcClientID string, includeState bool, signer []byte) (*elc.MsgUpdateClientResponse, error) {
	log.GetLogger().Info("UpdateClientStream1", "clientID", elcClientID, "includeState", includeState)
	stream, err := client.UpdateClientStream(ctx)
	if err != nil {
		return nil, err
	}
	log.GetLogger().Info("UpdateClientStream2", "clientID", elcClientID, "includeState", includeState)
	if err = stream.Send(&elc.MsgUpdateClientStreamChunk{
		Chunk: &elc.MsgUpdateClientStreamChunk_Init{
			Init: &elc.UpdateClientStreamInit{
				ClientId:     elcClientID,
				IncludeState: includeState,
				Signer:       signer,
				TypeUrl:      anyHeader.TypeUrl,
			},
		},
	}); err != nil {
		return nil, err
	}
	log.GetLogger().Info("UpdateClientStream3", "clientID", elcClientID, "includeState", includeState)
	chunks := split(anyHeader.Value, 3*1024*1024)
	for i, chunk := range chunks {
		err = stream.Send(&elc.MsgUpdateClientStreamChunk{
			Chunk: &elc.MsgUpdateClientStreamChunk_HeaderChunk{
				HeaderChunk: &elc.UpdateClientStreamHeaderChunk{
					Data: chunk,
				},
			},
		})
		if err != nil {
			log.GetLogger().Error(fmt.Sprintf("chunk failed: index = %d", i), err)
			return nil, err
		}
		log.GetLogger().Info("UpdateClientStream4", "clientID", elcClientID, "includeState", includeState)
	}
	log.GetLogger().Info("UpdateClientStream5", "clientID", elcClientID, "includeState", includeState)
	return stream.CloseAndRecv()
}

func split(buf []byte, lim int) [][]byte {
	var chunk []byte
	chunks := make([][]byte, 0, len(buf)/lim+1)
	for len(buf) >= lim {
		chunk, buf = buf[:lim], buf[lim:]
		chunks = append(chunks, chunk)
	}
	if len(buf) > 0 {
		chunks = append(chunks, buf[:len(buf)])
	}
	return chunks
}
