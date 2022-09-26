package boltedsftp

import (
	"context"
	"crypto/rsa"
	"fmt"
	"io"
	"net"

	"github.com/draganm/bolted"
	"github.com/go-logr/logr"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func Serve(ctx context.Context, addr string, db bolted.Database, pk *rsa.PrivateKey, log logr.Logger) (string, error) {
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			fmt.Println("password", string(password))
			return &ssh.Permissions{}, nil
		},
	}

	hostSigner, err := ssh.NewSignerFromKey(pk)
	if err != nil {
		return "", fmt.Errorf("while creating signer from private key: %w", err)
	}
	cfg.AddHostKey(hostSigner)

	l, err := net.Listen("tcp", addr)
	if err != nil {
		return "", fmt.Errorf("while creating listener: %w", err)
	}

	go func() {
		<-ctx.Done()
		err = l.Close()
		if err != nil {
			log.Error(err, "while closing listener")
		}
	}()
	go func() {

		for {
			conn, err := l.Accept()

			if err != nil {
				log.Error(err, "failed to accept sftp connection")
			}
			go handleConnection(ctx, cfg, conn, db, log)

		}
	}()

	return l.Addr().String(), nil
}

func handleConnection(ctx context.Context, cfg *ssh.ServerConfig, conn net.Conn, db bolted.Database, log logr.Logger) (err error) {

	defer func() {
		if err != nil {
			log.WithValues("remoteAddr", conn.RemoteAddr().String()).Error(err, "while serving sftp")
		}
	}()

	_, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return fmt.Errorf("while starting server handshake: %w", err)
	}

	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		// TODO: new go routine for this?

		// Channels have a type, depending on the application level
		// protocol intended. In the case of an SFTP session, this is "subsystem"
		// with a payload string of "<length=4>sftp"
		if newChannel.ChannelType() != "session" {

			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		requestChannel, requests, err := newChannel.Accept()
		if err != nil {
			return fmt.Errorf("could not accept channel: %w", err)
		}

		// Sessions have out-of-band requests such as "shell",
		// "pty-req" and "env".  Here we handle only the
		// "subsystem" request.

		go func(in <-chan *ssh.Request) {
			for req := range in {
				ok := false
				switch req.Type {
				case "subsystem":
					if string(req.Payload[4:]) == "sftp" {
						ok = true
					}
				}
				req.Reply(ok, nil)
			}
		}(requests)

		// serverOptions := []sftp.ServerOption{
		// 	// sftp.WithDebug(debugStream),
		// }

		// if readOnly {
		// 	serverOptions = append(serverOptions, sftp.ReadOnly())
		// }

		dbh := &dbHandler{Database: db}

		server := sftp.NewRequestServer(requestChannel, sftp.Handlers{
			FileGet:  dbh,
			FilePut:  dbh,
			FileCmd:  dbh,
			FileList: dbh,
		})

		err = server.Serve()
		if err == io.EOF {
			server.Close()
		}

		fmt.Println("serving done")

		if err != nil {
			return fmt.Errorf("server failed: %w", err)
		}
	}

	return nil

}
