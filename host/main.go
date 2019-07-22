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

type GetterStreamResponse interface {
	Get2(msg string) (chan string, error)
}

type PluginConnector struct {
	plugin.Plugin
}

func (*PluginConnector) GRPCServer(*plugin.GRPCBroker, *grpc.Server) error {
	panic("implement me")
}

func (h *PluginConnector) GRPCClient(ctx context.Context, b *plugin.GRPCBroker, cc *grpc.ClientConn) (interface{}, error) {
	return &pluginGateway{
		client: proto.NewEchoServiceClient(cc),
	}, nil
}

type pluginGateway struct {
	client proto.EchoServiceClient
}

func (a *pluginGateway) Set(stream chan string) (string, error) {
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

func (a *pluginGateway) Get(msg string, rev chan string) error {
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

func (a *pluginGateway) Get2(msg string) (chan string, error) {
	streamer, err := a.client.Get(context.Background(), &proto.EchoMsg{Msg: msg})
	if err != nil {
		return nil, err
	}
	rev := make(chan string)
	go func() {
		for {
			resp, err := streamer.Recv()
			if err == io.EOF {
				close(rev)
				err = streamer.CloseSend()
				log.Println(err)
				break
			}
			if err != nil {
				close(rev)
				log.Println(err)
				break
			}
			rev <- resp.Msg
		}
	}()
	return rev, nil
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
				"p": &PluginConnector{},
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

func (s *pluginService) GetterStreamResponse() (GetterStreamResponse, error) {
	c, err := s.client.Client()
	if err != nil {
		return nil, err
	}
	p, err := c.Dispense("p")
	if err != nil {
		return nil, err
	}
	return p.(GetterStreamResponse), nil
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

	log.Println("Getting stream responses 2")
	getStreamer2, err := service.GetterStreamResponse()
	if err != nil {
		log.Fatal(err)
	}
	revChan, err := getStreamer2.Get2("Hello world")
	if err != nil {
		log.Fatal(err)
	}
	for m := range revChan {
		log.Println("stream: ", m)
	}
}
