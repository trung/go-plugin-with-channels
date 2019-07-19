package main

import (
	"context"
	"io"
	"log"
	"os"
	"os/exec"

	"github.com/hashicorp/go-plugin"
	"github.com/trung/go-plugin-with-channels/proto"
	"google.golang.org/grpc"
)

type Getter interface {
	Get(msg string, rev chan string) error
}

type Setter interface {
	Set(stream chan string) (string, error)
}

type PluginGateway struct {
	plugin.Plugin
}

func (*PluginGateway) GRPCServer(*plugin.GRPCBroker, *grpc.Server) error {
	panic("implement me")
}

func (h *PluginGateway) GRPCClient(ctx context.Context, b *plugin.GRPCBroker, cc *grpc.ClientConn) (interface{}, error) {
	return &pluginAdapter{
		client: proto.NewEchoServiceClient(cc),
	}, nil
}

type pluginAdapter struct {
	client proto.EchoServiceClient
}

func (a *pluginAdapter) Set(stream chan string) (string, error) {
	streamer, err := a.client.Set(context.Background())
	if err != nil {
		return "", err
	}
	for m := range stream {
		if err := streamer.Send(&proto.EchoMsg{Msg: m}); err != nil {
			return "", err
		}
	}
	resp, err := streamer.CloseAndRecv()
	if err != nil {
		return "", err
	}
	return resp.Msg, nil
}

func (a *pluginAdapter) Get(msg string, rev chan string) error {
	streamer, err := a.client.Get(context.Background(), &proto.EchoMsg{Msg: msg})
	if err != nil {
		return err
	}
	for {
		resp, err := streamer.Recv()
		if err == io.EOF {
			close(rev)
			return streamer.CloseSend()
		}
		if err != nil {
			return err
		}
		rev <- resp.Msg
	}
}

type pluginService struct {
	client *plugin.Client
}

func newPluginService() *pluginService {
	return &pluginService{
		client: plugin.NewClient(&plugin.ClientConfig{
			HandshakeConfig: plugin.HandshakeConfig{
				ProtocolVersion:  1,
				MagicCookieValue: "V",
				MagicCookieKey:   "K",
			},
			Plugins: map[string]plugin.Plugin{
				"p": &PluginGateway{},
			},
			AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
			Cmd:              exec.Command(os.Getenv("PLUGIN_CMD")),
		}),
	}
}

func (s *pluginService) Start() (err error) {
	_, err = s.client.Client()
	if err != nil {
		return
	}
	return nil
}

func (s *pluginService) Stop() error {
	s.client.Kill()
	return nil
}

func (s *pluginService) Getter() (Getter, error) {
	c, err := s.client.Client()
	if err != nil {
		return nil, err
	}
	p, err := c.Dispense("p")
	if err != nil {
		return nil, err
	}
	return p.(Getter), nil
}

func (s *pluginService) Setter() (Setter, error) {
	c, err := s.client.Client()
	if err != nil {
		return nil, err
	}
	p, err := c.Dispense("p")
	if err != nil {
		return nil, err
	}
	return p.(Setter), nil
}

func main() {
	service := newPluginService()
	if err := service.Start(); err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = service.Stop()
	}()
	log.Println("Getting stream responses")
	getStreamer, err := service.Getter()
	if err != nil {
		log.Fatal(err)
	}
	rev := make(chan string)
	go func() {
		if err := getStreamer.Get("Hello World", rev); err != nil {
			log.Fatal(err)
		}
	}()
	for m := range rev {
		log.Println("stream: ", m)
	}
	log.Println("Setting stream requests")
	setStreamer, err := service.Setter()
	if err != nil {
		log.Fatal(err)
	}
	send := make(chan string)
	go func() {
		strings := []string{"H", "e", "l", "l", "o"}
		log.Println("Send: ", strings)
		for _, m := range strings {
			send <- m
		}
		close(send)
	}()
	msg, err := setStreamer.Set(send)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Rev: ", msg)
}
