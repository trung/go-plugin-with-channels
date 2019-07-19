package main

import (
	"context"
	"fmt"
	"io"

	"github.com/hashicorp/go-plugin"
	"github.com/trung/go-plugin-with-channels/proto"
	"google.golang.org/grpc"
)

type pluginServer struct {
	plugin.Plugin
}

func (*pluginServer) Get(msg *proto.EchoMsg, g proto.EchoService_GetServer) error {
	for _, m := range msg.Msg {
		if err := g.Send(&proto.EchoMsg{Msg: fmt.Sprintf("%c", m)}); err != nil {
			return err
		}
	}
	return nil
}

func (*pluginServer) Set(s proto.EchoService_SetServer) error {
	var msg string
	for {
		req, err := s.Recv()
		if err == io.EOF {
			return s.SendAndClose(&proto.EchoMsg{Msg: msg})
		}
		if err != nil {
			return err
		}
		msg = msg + req.Msg
	}
}

func (p *pluginServer) GRPCServer(b *plugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterEchoServiceServer(s, p)
	return nil
}

func (*pluginServer) GRPCClient(context.Context, *plugin.GRPCBroker, *grpc.ClientConn) (interface{}, error) {
	panic("implement me")
}

func main() {
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: plugin.HandshakeConfig{
			ProtocolVersion:  1,
			MagicCookieValue: "V",
			MagicCookieKey:   "K",
		},
		Plugins: map[string]plugin.Plugin{
			"p": &pluginServer{},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
