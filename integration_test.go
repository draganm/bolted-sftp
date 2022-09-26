package boltedsftp_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/cucumber/godog"
	"github.com/draganm/bolted"
	boltedsftp "github.com/draganm/bolted-sftp"
	"github.com/draganm/bolted/dbpath"
	"github.com/draganm/bolted/embedded"
	"github.com/go-logr/logr"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			TestingT: t, // Testing instance that will run subtests.
			NoColors: true,
		},
	}

	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}

func InitializeScenario(ctx *godog.ScenarioContext) {
	err := initializeScenario(ctx)
	if err != nil {
		panic(fmt.Errorf("failed to initialize scenario: %w", err))
	}
}

func initializeScenario(ctx *godog.ScenarioContext) error {
	ti := &testInstance{}
	var td string
	var err error
	var db bolted.Database
	ctx.Before(func(ctx context.Context, scen *godog.Scenario) (context.Context, error) {
		td, err = os.MkdirTemp("", "")
		if err != nil {
			return ctx, fmt.Errorf("while opening temp dir: %w", err)
		}

		db, err = embedded.Open(filepath.Join(td, "data"), 0700, embedded.Options{})
		if err != nil {
			return ctx, fmt.Errorf("while opening db: %w", err)
		}

		ti.db = db

		return ctx, nil
	})

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		pk, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return ctx, err
		}
		cfg := &ssh.ServerConfig{
			PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
				return &ssh.Permissions{}, nil
			},
			PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
				return &ssh.Permissions{}, nil
			},
		}

		hostSigner, err := ssh.NewSignerFromKey(pk)
		if err != nil {
			return ctx, fmt.Errorf("while creating signer from private key: %w", err)
		}
		cfg.AddHostKey(hostSigner)

		addr, err := boltedsftp.Serve(ctx, "localhost:0", db, cfg, logr.Discard())
		if err != nil {
			return ctx, err
		}

		ti.serverAddr = addr

		return ctx, nil

	})

	ctx.Before(func(ctx context.Context, scn *godog.Scenario) (context.Context, error) {
		conn, err := ssh.Dial("tcp", ti.serverAddr, &ssh.ClientConfig{
			User: "testuser",
			HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
				return nil
			},
			Auth: []ssh.AuthMethod{ssh.Password("testpassword")},
		})

		if err != nil {
			return ctx, fmt.Errorf("while opening ssh client: %w", err)
		}

		sc, err := sftp.NewClient(conn)
		if err != nil {
			return ctx, fmt.Errorf("while creating sftp client: %w", err)
		}

		ti.sc = sc

		return ctx, nil
	})

	ctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		if ti.db != nil {
			return ctx, ti.db.Close()
		}
		return ctx, err
	})
	ctx.Step(`^an empty database$`, ti.anEmptyDatabase)
	ctx.Step(`^I list the root directory$`, ti.iListTheRootDirectory)
	ctx.Step(`^the result should be empty$`, ti.theResultShouldBeEmpty)
	ctx.Step(`^a database with one map in the root$`, ti.aDatabaseWithOneMapInTheRoot)
	ctx.Step(`^the result should have one directory$`, ti.theResultShouldHaveOneDirectory)
	ctx.Step(`^a database with (\d+) maps in the root$`, ti.aDatabaseWithMapsInTheRoot)
	ctx.Step(`^the result should have (\d+) directories$`, ti.theResultShouldHaveDirectories)
	ctx.Step(`^file with some database$`, ti.fileWithSomeDatabase)
	ctx.Step(`^I fetch the file$`, ti.iFetchTheFile)
	ctx.Step(`^I should get the content of the file$`, ti.iShouldGetTheContentOfTheFile)
	ctx.Step(`^I list the map directory$`, ti.iListTheMapDirectory)
	ctx.Step(`^the map in the root contains one submap$`, ti.theMapInTheRootContainsOneSubmap)

	return nil
}

type testInstance struct {
	db         bolted.Database
	serverAddr string
	sc         *sftp.Client
	files      []os.FileInfo
	data       []byte
}

func (ti *testInstance) anEmptyDatabase() error {
	// it's already empty at the beginning of the test
	return nil
}

func (ti *testInstance) iListTheRootDirectory() error {
	fi, err := ti.sc.ReadDir("/")
	if err != nil {
		return err
	}
	ti.files = fi
	return nil
}

func (ti *testInstance) theResultShouldBeEmpty() error {
	if len(ti.files) != 0 {
		return fmt.Errorf("expected %d files, but got %d", 0, len(ti.files))
	}
	return nil
}

func (ti *testInstance) aDatabaseWithOneMapInTheRoot() error {
	return bolted.SugaredWrite(ti.db, func(tx bolted.SugaredWriteTx) error {
		tx.CreateMap(dbpath.ToPath("foo"))
		return nil
	})
}

func (ti *testInstance) theResultShouldHaveOneDirectory() error {
	if len(ti.files) != 1 {
		return fmt.Errorf("expected %d files, but got %d", 1, len(ti.files))
	}

	return nil
}

func (ti *testInstance) aDatabaseWithMapsInTheRoot(cnt int) error {
	return bolted.SugaredWrite(ti.db, func(tx bolted.SugaredWriteTx) error {
		for i := 0; i < cnt; i++ {
			tx.CreateMap(dbpath.ToPath(fmt.Sprintf("%05d", i)))
		}
		return nil
	})
}

func (ti *testInstance) theResultShouldHaveDirectories(cnt int) error {
	if len(ti.files) != cnt {
		return fmt.Errorf("expected %d files, but got %d", cnt, len(ti.files))
	}

	return nil
}

func (ti *testInstance) fileWithSomeDatabase() error {
	return bolted.SugaredWrite(ti.db, func(tx bolted.SugaredWriteTx) error {
		tx.Put(dbpath.ToPath("foo"), []byte("bar"))
		return nil
	})
}

func (ti *testInstance) iFetchTheFile() error {
	f, err := ti.sc.Open("foo")
	if err != nil {
		return err
	}

	defer f.Close()
	ti.data, err = io.ReadAll(f)
	if err != nil {
		return err
	}

	return nil

}

func (ti *testInstance) iShouldGetTheContentOfTheFile() error {
	if string(ti.data) != "bar" {
		return fmt.Errorf("expected %q but got %q", "bar", string(ti.data))
	}
	return nil
}

func (ti *testInstance) iListTheMapDirectory() error {
	fi, err := ti.sc.ReadDir("/foo")
	if err != nil {
		return err
	}
	ti.files = fi
	return nil
}

func (ti *testInstance) theMapInTheRootContainsOneSubmap() error {
	return bolted.SugaredWrite(ti.db, func(tx bolted.SugaredWriteTx) error {
		tx.CreateMap(dbpath.ToPath("foo", "bar"))
		return nil
	})
}
