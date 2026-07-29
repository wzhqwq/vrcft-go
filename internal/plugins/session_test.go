package plugins

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/ipc"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/protocol"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

const pluginSessionTestTimeout = 2 * time.Second

type sessionTestListener struct {
	conn protocol.Conn
	err  error

	mu         sync.Mutex
	accepted   bool
	closeCount int
	deadline   time.Time
}

func (l *sessionTestListener) Accept(ctx context.Context) (protocol.Conn, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.accepted {
		return nil, errors.New("test listener already accepted")
	}
	l.accepted = true
	l.deadline, _ = ctx.Deadline()
	return l.conn, l.err
}

func (l *sessionTestListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closeCount++
	return nil
}

func (l *sessionTestListener) isClosed() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.closeCount > 0
}

func (l *sessionTestListener) closes() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.closeCount
}

type sessionTestProcess struct {
	wait chan error

	mu            sync.Mutex
	killCount     int
	unblockOnKill bool
}

func newSessionTestProcess() *sessionTestProcess {
	return &sessionTestProcess{wait: make(chan error, 1), unblockOnKill: true}
}

func (p *sessionTestProcess) PID() int { return 4242 }
func (p *sessionTestProcess) Wait() error {
	return <-p.wait
}
func (p *sessionTestProcess) Kill() error {
	p.mu.Lock()
	p.killCount++
	unblock := p.unblockOnKill
	p.mu.Unlock()
	if unblock {
		select {
		case p.wait <- errors.New("test process killed"):
		default:
		}
	}
	return nil
}

func (p *sessionTestProcess) kills() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.killCount
}

type sessionTestLauncher struct {
	start func(context.Context, ProcessSpec) (Process, error)
}

func (l sessionTestLauncher) Start(ctx context.Context, spec ProcessSpec) (Process, error) {
	return l.start(ctx, spec)
}

func TestPluginSessionListensBeforeLaunchAndCompletesRealHandshake(t *testing.T) {
	hostRaw, pluginRaw := net.Pipe()
	hostConn := ipc.WrapConn(hostRaw)
	pluginConn := ipc.WrapConn(pluginRaw)
	t.Cleanup(func() {
		_ = hostConn.Close()
		_ = pluginConn.Close()
	})

	listener := &sessionTestListener{conn: hostConn}
	process := newSessionTestProcess()
	pipeName := "session-fixed-pipe"
	token := handshakeToken(21)
	var (
		mu             sync.Mutex
		order          []string
		started        ProcessSpec
		pluginErr      = make(chan error, 1)
		processStarted = make(chan int, 1)
		ready          = make(chan struct{}, 1)
	)

	dependencies := sessionDependencies{
		credentials: func() (string, string, error) {
			return pipeName, token, nil
		},
		listen: func(config ipc.ServerConfig) (ipc.Listener, error) {
			mu.Lock()
			order = append(order, "listen")
			mu.Unlock()
			if config.PipeName != pipeName {
				t.Errorf("Listen PipeName = %q, want %q", config.PipeName, pipeName)
			}
			return listener, nil
		},
		launcher: sessionTestLauncher{start: func(_ context.Context, spec ProcessSpec) (Process, error) {
			mu.Lock()
			order = append(order, "launch")
			started = spec
			mu.Unlock()
			go func() {
				if err := pluginConn.Send(context.Background(), validHello(token)); err != nil {
					pluginErr <- err
					return
				}
				message, err := pluginConn.Receive(context.Background())
				if err != nil {
					pluginErr <- err
					return
				}
				initialize, ok := message.Payload.(protocol.Initialize)
				if !ok {
					pluginErr <- errors.New("host did not send Initialize")
					return
				}
				if !reflect.DeepEqual(initialize.Startup, validHandshakeStartup()) {
					pluginErr <- errors.New("Initialize did not contain the startup snapshot")
					return
				}
				ready, err := protocol.NewMessage(protocol.Ready{})
				if err == nil {
					err = pluginConn.Send(context.Background(), ready)
				}
				pluginErr <- err
			}()
			return process, nil
		}},
		onProcessStarted: func(instanceID uint64, pid int) {
			if instanceID != 7 {
				t.Errorf("ProcessStarted instance ID = %d, want 7", instanceID)
			}
			processStarted <- pid
		},
		onReady: func(instanceID uint64) {
			if instanceID != 7 {
				t.Errorf("Ready instance ID = %d, want 7", instanceID)
			}
			ready <- struct{}{}
		},
	}
	config := sessionConfig{
		Plugin: InstalledPlugin{
			Manifest:   validManifest(),
			RootDir:    `C:\plugins\camera`,
			Executable: `C:\plugins\camera\plugin.exe`,
		},
		Startup:          validHandshakeStartup(),
		HandshakeTimeout: time.Second,
		HeartbeatTimeout: time.Minute,
		GracefulTimeout:  time.Second,
		KillTimeout:      time.Second,
		ControlCapacity:  4,
	}

	session := newPluginSession(context.Background(), 7, config, dependencies)
	if pid := awaitValue(t, processStarted); pid != process.PID() {
		t.Fatalf("ProcessStarted PID = %d, want %d", pid, process.PID())
	}
	select {
	case err := <-pluginErr:
		if err != nil {
			t.Fatalf("plugin handshake error = %v", err)
		}
	case <-time.After(pluginSessionTestTimeout):
		t.Fatal("plugin handshake did not complete")
	}
	awaitValue(t, ready)

	mu.Lock()
	gotOrder := append([]string(nil), order...)
	gotSpec := started
	mu.Unlock()
	if !reflect.DeepEqual(gotOrder, []string{"listen", "launch"}) {
		t.Fatalf("startup order = %v, want [listen launch]", gotOrder)
	}
	if gotSpec.Executable != config.Plugin.Executable || gotSpec.WorkingDir != config.Plugin.RootDir || len(gotSpec.Args) != 0 {
		t.Fatalf("ProcessSpec = %#v, want executable/root and no arguments", gotSpec)
	}
	wantEnvironment, err := launchEnvironment(os.Environ(), pipeName, token)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotSpec.Env, wantEnvironment) {
		t.Fatalf("ProcessSpec.Env does not contain the exact launch environment")
	}
	if !listener.isClosed() {
		t.Fatal("listener remains open after Accept")
	}

	process.wait <- nil
	result := awaitSessionResult(t, session.Done())
	assertUnexpectedExitResult(t, result)
	if result.StartedAt.IsZero() {
		t.Fatal("session StartedAt is zero after successful handshake")
	}
}

func TestPluginSessionOwnsStartupConfigBeforeAsyncLaunch(t *testing.T) {
	hostRaw, pluginRaw := net.Pipe()
	hostConn := ipc.WrapConn(hostRaw)
	pluginConn := ipc.WrapConn(pluginRaw)
	t.Cleanup(func() {
		_ = hostConn.Close()
		_ = pluginConn.Close()
	})
	listener := &sessionTestListener{conn: hostConn}
	process := newSessionTestProcess()
	token := handshakeToken(31)
	startEntered := make(chan struct{})
	releaseStart := make(chan struct{})
	initializeResult := make(chan pluginapi.Startup, 1)
	startup := validHandshakeStartup()
	session := newPluginSession(context.Background(), 22, sessionConfig{
		Plugin: InstalledPlugin{
			Manifest:   validManifest(),
			RootDir:    `C:\plugins\camera`,
			Executable: `C:\plugins\camera\plugin.exe`,
		},
		Startup:          startup,
		HandshakeTimeout: time.Second,
		HeartbeatTimeout: time.Minute,
		GracefulTimeout:  30 * time.Millisecond,
		KillTimeout:      30 * time.Millisecond,
		ControlCapacity:  2,
	}, sessionDependencies{
		credentials: func() (string, string, error) {
			return "session-startup-ownership", token, nil
		},
		listen: func(ipc.ServerConfig) (ipc.Listener, error) {
			return listener, nil
		},
		launcher: sessionTestLauncher{start: func(context.Context, ProcessSpec) (Process, error) {
			close(startEntered)
			<-releaseStart
			go func() {
				_ = pluginConn.Send(context.Background(), validHello(token))
				message, err := pluginConn.Receive(context.Background())
				if err != nil {
					return
				}
				initializeResult <- message.Payload.(protocol.Initialize).Startup
				ready, _ := protocol.NewMessage(protocol.Ready{})
				_ = pluginConn.Send(context.Background(), ready)
			}()
			return process, nil
		}},
	})
	awaitValue(t, startEntered)
	copy(startup.Config.Data, []byte(`{"gain":9.9}`))
	close(releaseStart)
	initialize := awaitValue(t, initializeResult)
	if got := string(initialize.Config.Data); got != `{"gain":0.5}` {
		t.Fatalf("Initialize Config.Data = %q after caller mutation, want owned bytes", got)
	}
	process.wait <- nil
	assertUnexpectedExitResult(t, awaitSessionResult(t, session.Done()))
}

func TestPluginSessionStartupFailuresCleanOwnedResources(t *testing.T) {
	startErr := errors.New("start failure")
	acceptErr := errors.New("accept failure")
	for _, test := range []struct {
		name        string
		credentials func() (string, string, error)
		listenErr   error
		startErr    error
		acceptErr   error
		wantListen  bool
		wantLaunch  bool
		wantKill    bool
	}{
		{
			name:        "credentials",
			credentials: func() (string, string, error) { return "", "", errors.New("entropy failure") },
		},
		{name: "listen", listenErr: errors.New("listen failure"), wantListen: true},
		{name: "launch", startErr: startErr, wantListen: true, wantLaunch: true},
		{
			name:       "accept",
			acceptErr:  acceptErr,
			wantListen: true,
			wantLaunch: true,
			wantKill:   true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			process := newSessionTestProcess()
			listener := &sessionTestListener{err: test.acceptErr}
			var listenCalls, launchCalls int
			credentials := test.credentials
			if credentials == nil {
				credentials = func() (string, string, error) {
					return "session-startup-failure", handshakeToken(26), nil
				}
			}
			session := newPluginSession(context.Background(), 16, sessionConfig{
				Plugin: InstalledPlugin{
					Manifest:   validManifest(),
					RootDir:    `C:\plugins\camera`,
					Executable: `C:\plugins\camera\plugin.exe`,
				},
				Startup:          validHandshakeStartup(),
				HandshakeTimeout: 20 * time.Millisecond,
				HeartbeatTimeout: time.Minute,
				GracefulTimeout:  20 * time.Millisecond,
				KillTimeout:      20 * time.Millisecond,
				ControlCapacity:  2,
			}, sessionDependencies{
				credentials: credentials,
				listen: func(ipc.ServerConfig) (ipc.Listener, error) {
					listenCalls++
					if test.listenErr != nil {
						return nil, test.listenErr
					}
					return listener, nil
				},
				launcher: sessionTestLauncher{start: func(context.Context, ProcessSpec) (Process, error) {
					launchCalls++
					if test.startErr != nil {
						return nil, test.startErr
					}
					return process, nil
				}},
			})
			result := awaitSessionResult(t, session.Done())
			if result.Err == nil {
				t.Fatal("startup result error = nil")
			}
			if got := listenCalls; (got == 1) != test.wantListen {
				t.Fatalf("listen calls = %d", got)
			}
			if got := launchCalls; (got == 1) != test.wantLaunch {
				t.Fatalf("launch calls = %d", got)
			}
			if listener != nil && test.listenErr == nil && test.wantListen {
				if got := listener.closes(); got != 1 {
					t.Fatalf("listener Close calls = %d, want 1", got)
				}
			}
			if got := process.kills(); (got == 1) != test.wantKill {
				t.Fatalf("process Kill calls = %d, wantKill %v", got, test.wantKill)
			}
		})
	}
}

func TestPluginSessionClassifiesFailureAtItsSource(t *testing.T) {
	t.Run("accept transport remains retryable", func(t *testing.T) {
		process := newSessionTestProcess()
		session := newPluginSession(context.Background(), 31, sourceClassificationSessionConfig(), sessionDependencies{
			credentials: sourceClassificationCredentials,
			listen: func(ipc.ServerConfig) (ipc.Listener, error) {
				return &sessionTestListener{err: os.ErrNotExist}, nil
			},
			launcher: sessionTestLauncher{start: func(context.Context, ProcessSpec) (Process, error) {
				return process, nil
			}},
		})
		result := awaitSessionResult(t, session.Done())
		if !result.Retryable || !errors.Is(result.Err, os.ErrNotExist) ||
			errors.Is(result.Err, ErrProtocolViolation) {
			t.Fatalf("Accept result = %+v, want retryable transport without protocol violation", result)
		}
	})

	t.Run("handshake transport remains retryable", func(t *testing.T) {
		process := newSessionTestProcess()
		session := newPluginSession(context.Background(), 32, sourceClassificationSessionConfig(), sessionDependencies{
			credentials: sourceClassificationCredentials,
			listen: func(ipc.ServerConfig) (ipc.Listener, error) {
				return &sessionTestListener{conn: failingHandshakeConn{err: os.ErrNotExist}}, nil
			},
			launcher: sessionTestLauncher{start: func(context.Context, ProcessSpec) (Process, error) {
				return process, nil
			}},
		})
		result := awaitSessionResult(t, session.Done())
		if !result.Retryable || !errors.Is(result.Err, os.ErrNotExist) ||
			errors.Is(result.Err, ErrProtocolViolation) {
			t.Fatalf("handshake transport result = %+v, want retryable transport without protocol violation", result)
		}
	})

	t.Run("semantic handshake violation is not retryable", func(t *testing.T) {
		hostRaw, pluginRaw := net.Pipe()
		hostConn := ipc.WrapConn(hostRaw)
		pluginConn := ipc.WrapConn(pluginRaw)
		t.Cleanup(func() {
			_ = hostConn.Close()
			_ = pluginConn.Close()
		})
		process := newSessionTestProcess()
		session := newPluginSession(context.Background(), 33, sourceClassificationSessionConfig(), sessionDependencies{
			credentials: sourceClassificationCredentials,
			listen: func(ipc.ServerConfig) (ipc.Listener, error) {
				return &sessionTestListener{conn: hostConn}, nil
			},
			launcher: sessionTestLauncher{start: func(context.Context, ProcessSpec) (Process, error) {
				go func() {
					ready, _ := protocol.NewMessage(protocol.Ready{})
					_ = pluginConn.Send(context.Background(), ready)
				}()
				return process, nil
			}},
		})
		result := awaitSessionResult(t, session.Done())
		if result.Retryable || !errors.Is(result.Err, ErrProtocolViolation) {
			t.Fatalf("semantic handshake result = %+v, want non-retryable protocol violation", result)
		}
	})

	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "missing executable", err: classifiedStartError(startFailureExecutable, "not found", os.ErrNotExist)},
		{name: "working directory permission", err: classifiedStartError(startFailureWorkingDirectory, "access denied", os.ErrPermission)},
	} {
		t.Run(test.name, func(t *testing.T) {
			session := newPluginSession(context.Background(), 34, sourceClassificationSessionConfig(), sessionDependencies{
				credentials: sourceClassificationCredentials,
				listen: func(ipc.ServerConfig) (ipc.Listener, error) {
					return &sessionTestListener{}, nil
				},
				launcher: sessionTestLauncher{start: func(context.Context, ProcessSpec) (Process, error) {
					return nil, test.err
				}},
			})
			result := awaitSessionResult(t, session.Done())
			if result.Retryable || !errors.Is(result.Err, test.err) {
				t.Fatalf("launcher result = %+v, want source-classified non-retryable error", result)
			}
		})
	}

	t.Run("runtime IPC os sentinel remains retryable", func(t *testing.T) {
		hostRaw, pluginRaw := net.Pipe()
		hostConn := &sessionRuntimeFailureConn{
			Conn: ipc.WrapConn(hostRaw),
			err:  os.ErrPermission,
		}
		pluginConn := ipc.WrapConn(pluginRaw)
		t.Cleanup(func() {
			_ = hostConn.Close()
			_ = pluginConn.Close()
		})
		process := newSessionTestProcess()
		token := handshakeToken(44)
		session := newPluginSession(context.Background(), 35, sourceClassificationSessionConfig(), sessionDependencies{
			credentials: func() (string, string, error) {
				return "session-source-classification", token, nil
			},
			listen: func(ipc.ServerConfig) (ipc.Listener, error) {
				return &sessionTestListener{conn: hostConn}, nil
			},
			launcher: sessionTestLauncher{start: func(context.Context, ProcessSpec) (Process, error) {
				go func() {
					_ = pluginConn.Send(context.Background(), validHello(token))
					_, _ = pluginConn.Receive(context.Background())
					ready, _ := protocol.NewMessage(protocol.Ready{})
					_ = pluginConn.Send(context.Background(), ready)
					_, _ = pluginConn.Receive(context.Background())
					process.wait <- nil
				}()
				return process, nil
			}},
		})
		result := awaitSessionResult(t, session.Done())
		if !result.Retryable || !errors.Is(result.Err, os.ErrPermission) {
			t.Fatalf("runtime IPC result = %+v, want retryable transport sentinel", result)
		}
	})
}

func sourceClassificationSessionConfig() sessionConfig {
	return sessionConfig{
		Plugin: InstalledPlugin{
			Manifest:   validManifest(),
			RootDir:    `C:\plugins\camera`,
			Executable: `C:\plugins\camera\plugin.exe`,
		},
		Startup:          validHandshakeStartup(),
		HandshakeTimeout: time.Second,
		HeartbeatTimeout: time.Minute,
		GracefulTimeout:  20 * time.Millisecond,
		KillTimeout:      20 * time.Millisecond,
		ControlCapacity:  2,
	}
}

func sourceClassificationCredentials() (string, string, error) {
	return "session-source-classification", handshakeToken(43), nil
}

type sessionRuntimeFailureConn struct {
	protocol.Conn
	err error

	mu       sync.Mutex
	receives int
}

func (c *sessionRuntimeFailureConn) Receive(ctx context.Context) (protocol.Message, error) {
	c.mu.Lock()
	c.receives++
	receives := c.receives
	c.mu.Unlock()
	if receives > 2 {
		return protocol.Message{}, c.err
	}
	return c.Conn.Receive(ctx)
}

func TestPluginSessionHandshakeTimeoutClosesConnectionAndProcess(t *testing.T) {
	hostRaw, pluginRaw := net.Pipe()
	hostBase := ipc.WrapConn(hostRaw)
	pluginConn := ipc.WrapConn(pluginRaw)
	counted := &sessionCountingConn{Conn: hostBase}
	t.Cleanup(func() { _ = pluginConn.Close() })
	listener := &sessionTestListener{conn: counted}
	process := newSessionTestProcess()
	start := time.Now()
	session := newPluginSession(context.Background(), 17, sessionConfig{
		Plugin: InstalledPlugin{
			Manifest:   validManifest(),
			RootDir:    `C:\plugins\camera`,
			Executable: `C:\plugins\camera\plugin.exe`,
		},
		Startup:          validHandshakeStartup(),
		HandshakeTimeout: 25 * time.Millisecond,
		HeartbeatTimeout: time.Minute,
		GracefulTimeout:  20 * time.Millisecond,
		KillTimeout:      20 * time.Millisecond,
		ControlCapacity:  2,
	}, sessionDependencies{
		credentials: func() (string, string, error) {
			return "session-handshake-timeout", handshakeToken(27), nil
		},
		listen: func(ipc.ServerConfig) (ipc.Listener, error) {
			return listener, nil
		},
		launcher: sessionTestLauncher{start: func(context.Context, ProcessSpec) (Process, error) {
			return process, nil
		}},
	})
	result := awaitSessionResult(t, session.Done())
	if !errors.Is(result.Err, ErrHandshakeTimeout) {
		t.Fatalf("session error = %v, want ErrHandshakeTimeout", result.Err)
	}
	if listener.deadline.Before(start.Add(20 * time.Millisecond)) {
		t.Fatalf("Accept deadline = %v, want handshake deadline", listener.deadline)
	}
	if got := listener.closes(); got != 1 {
		t.Fatalf("listener Close calls = %d, want 1", got)
	}
	if got := counted.closes(); got != 1 {
		t.Fatalf("connection Close calls = %d, want 1", got)
	}
	if got := process.kills(); got != 1 {
		t.Fatalf("process Kill calls = %d, want 1", got)
	}
}

func TestPluginSessionStartupCleanupHonorsKillTimeout(t *testing.T) {
	t.Run("accept failure", func(t *testing.T) {
		process := newSessionTestProcess()
		process.mu.Lock()
		process.unblockOnKill = false
		process.mu.Unlock()
		session := newPluginSession(context.Background(), 26, sessionConfig{
			Plugin: InstalledPlugin{
				Manifest:   validManifest(),
				RootDir:    `C:\plugins\camera`,
				Executable: `C:\plugins\camera\plugin.exe`,
			},
			Startup:          validHandshakeStartup(),
			HandshakeTimeout: time.Second,
			HeartbeatTimeout: time.Minute,
			GracefulTimeout:  20 * time.Millisecond,
			KillTimeout:      20 * time.Millisecond,
			ControlCapacity:  2,
		}, sessionDependencies{
			credentials: func() (string, string, error) {
				return "session-accept-kill-timeout", handshakeToken(33), nil
			},
			listen: func(ipc.ServerConfig) (ipc.Listener, error) {
				return &sessionTestListener{err: errors.New("accept-secret")}, nil
			},
			launcher: sessionTestLauncher{start: func(context.Context, ProcessSpec) (Process, error) {
				return process, nil
			}},
		})
		result, ok := awaitBoundedStartupResult(session.Done(), 150*time.Millisecond)
		process.wait <- errors.New("cleanup")
		if !ok {
			t.Fatal("Accept failure blocked indefinitely in Process.Wait")
		}
		if !errors.Is(result.Err, ErrKillTimeout) {
			t.Fatalf("session error = %v, want ErrKillTimeout", result.Err)
		}
		if strings.Contains(result.Err.Error(), "accept-secret") {
			t.Fatalf("session error exposes accept cause: %v", result.Err)
		}
	})

	t.Run("handshake failure", func(t *testing.T) {
		hostRaw, pluginRaw := net.Pipe()
		hostConn := ipc.WrapConn(hostRaw)
		pluginConn := ipc.WrapConn(pluginRaw)
		t.Cleanup(func() {
			_ = hostConn.Close()
			_ = pluginConn.Close()
		})
		process := newSessionTestProcess()
		process.mu.Lock()
		process.unblockOnKill = false
		process.mu.Unlock()
		expectedToken := handshakeToken(34)
		session := newPluginSession(context.Background(), 27, sessionConfig{
			Plugin: InstalledPlugin{
				Manifest:   validManifest(),
				RootDir:    `C:\plugins\camera`,
				Executable: `C:\plugins\camera\plugin.exe`,
			},
			Startup:          validHandshakeStartup(),
			HandshakeTimeout: time.Second,
			HeartbeatTimeout: time.Minute,
			GracefulTimeout:  20 * time.Millisecond,
			KillTimeout:      20 * time.Millisecond,
			ControlCapacity:  2,
		}, sessionDependencies{
			credentials: func() (string, string, error) {
				return "session-handshake-kill-timeout", expectedToken, nil
			},
			listen: func(ipc.ServerConfig) (ipc.Listener, error) {
				return &sessionTestListener{conn: hostConn}, nil
			},
			launcher: sessionTestLauncher{start: func(context.Context, ProcessSpec) (Process, error) {
				go func() {
					_ = pluginConn.Send(context.Background(), validHello(handshakeToken(35)))
				}()
				return process, nil
			}},
		})
		result, ok := awaitBoundedStartupResult(session.Done(), 150*time.Millisecond)
		process.wait <- errors.New("cleanup")
		if !ok {
			t.Fatal("Handshake failure blocked indefinitely in Process.Wait")
		}
		if !errors.Is(result.Err, ErrAuthenticationFailed) || !errors.Is(result.Err, ErrKillTimeout) {
			t.Fatalf("session error = %v, want authentication and KillTimeout causes", result.Err)
		}
		if strings.Contains(result.Err.Error(), expectedToken) {
			t.Fatalf("session error exposes token: %v", result.Err)
		}
	})
}

func awaitBoundedStartupResult(done <-chan sessionResult, timeout time.Duration) (sessionResult, bool) {
	select {
	case result := <-done:
		return result, true
	case <-time.After(timeout):
		return sessionResult{}, false
	}
}

type sessionCountingConn struct {
	protocol.Conn
	mu         sync.Mutex
	closeCount int
}

func (c *sessionCountingConn) Close() error {
	c.mu.Lock()
	c.closeCount++
	c.mu.Unlock()
	return c.Conn.Close()
}

func (c *sessionCountingConn) closes() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeCount
}

type sessionTestFrame struct {
	pluginID   string
	generation uint64
	frame      trackingmodel.TrackingFrame
}

type sessionTestFrameSink struct {
	frames chan sessionTestFrame
}

func (s *sessionTestFrameSink) Submit(pluginID string, generation uint64, frame trackingmodel.TrackingFrame) {
	s.frames <- sessionTestFrame{pluginID: pluginID, generation: generation, frame: frame}
}

func TestPluginSessionRoutesRuntimeMessagesAndRejectsWrongDirection(t *testing.T) {
	for _, test := range []struct {
		name            string
		sendWrong       bool
		wantProtocolErr bool
	}{
		{name: "runtime observations"},
		{name: "wrong direction", sendWrong: true, wantProtocolErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			hostRaw, pluginRaw := net.Pipe()
			hostConn := ipc.WrapConn(hostRaw)
			pluginConn := ipc.WrapConn(pluginRaw)
			t.Cleanup(func() {
				_ = hostConn.Close()
				_ = pluginConn.Close()
			})
			listener := &sessionTestListener{conn: hostConn}
			process := newSessionTestProcess()
			token := handshakeToken(22)
			ready := make(chan struct{})
			sink := &sessionTestFrameSink{frames: make(chan sessionTestFrame, 1)}
			heartbeats := make(chan uint64, 1)
			statuses := make(chan pluginapi.DeviceStatus, 1)
			logs := make(chan pluginapi.LogEntry, 1)
			dependencies := sessionDependencies{
				credentials: func() (string, string, error) {
					return "session-runtime-pipe", token, nil
				},
				listen: func(ipc.ServerConfig) (ipc.Listener, error) {
					return listener, nil
				},
				launcher: sessionTestLauncher{start: func(context.Context, ProcessSpec) (Process, error) {
					go func() {
						if err := pluginConn.Send(context.Background(), validHello(token)); err != nil {
							return
						}
						if _, err := pluginConn.Receive(context.Background()); err != nil {
							return
						}
						message, _ := protocol.NewMessage(protocol.Ready{})
						if pluginConn.Send(context.Background(), message) == nil {
							close(ready)
						}
					}()
					return process, nil
				}},
				frameSink: sink,
				onHeartbeat: func(instanceID uint64, _ time.Time) {
					heartbeats <- instanceID
				},
				onStatus: func(instanceID uint64, status pluginapi.DeviceStatus) {
					if instanceID != 8 {
						t.Errorf("status instance ID = %d, want 8", instanceID)
					}
					statuses <- status
				},
				onLog: func(instanceID uint64, entry pluginapi.LogEntry) {
					if instanceID != 8 {
						t.Errorf("log instance ID = %d, want 8", instanceID)
					}
					logs <- entry
				},
			}
			config := sessionConfig{
				Plugin: InstalledPlugin{
					Manifest:   validManifest(),
					RootDir:    `C:\plugins\camera`,
					Executable: `C:\plugins\camera\plugin.exe`,
				},
				Startup:          validHandshakeStartup(),
				HandshakeTimeout: time.Second,
				HeartbeatTimeout: time.Minute,
				GracefulTimeout:  50 * time.Millisecond,
				KillTimeout:      50 * time.Millisecond,
				ControlCapacity:  4,
			}
			session := newPluginSession(context.Background(), 8, config, dependencies)
			select {
			case <-ready:
			case <-time.After(pluginSessionTestTimeout):
				t.Fatal("plugin did not become ready")
			}

			if test.sendWrong {
				sendSessionPayload(t, pluginConn, protocol.ConfigChanged{
					Config: pluginapi.Config{Revision: 2, Data: []byte(`{"secret":"wrong-direction"}`)},
				})
				result := awaitSessionResult(t, session.Done())
				if !errors.Is(result.Err, ErrProtocolViolation) {
					t.Fatalf("session error = %v, want ErrProtocolViolation", result.Err)
				}
				return
			}

			frame := trackingmodel.TrackingFrame{Sequence: 31, TimestampNS: 99}
			sendSessionPayload(t, pluginConn, protocol.Heartbeat{UptimeMS: 123})
			sendSessionPayload(t, pluginConn, protocol.TrackingFrame{Generation: 12, Frame: frame})
			sendSessionPayload(t, pluginConn, protocol.Status{
				Status: pluginapi.DeviceStatus{State: pluginapi.DeviceReady},
			})
			sendSessionPayload(t, pluginConn, protocol.Log{Level: pluginapi.LogInfo, Message: "device ready"})

			if got := awaitValue(t, heartbeats); got != 8 {
				t.Fatalf("heartbeat instance ID = %d, want 8", got)
			}
			gotFrame := awaitValue(t, sink.frames)
			if gotFrame.pluginID != config.Plugin.Manifest.ID || gotFrame.generation != 12 || gotFrame.frame != frame {
				t.Fatalf("FrameSink call = %#v", gotFrame)
			}
			if got := awaitValue(t, statuses); got != (pluginapi.DeviceStatus{State: pluginapi.DeviceReady}) {
				t.Fatalf("status = %#v", got)
			}
			if got := awaitValue(t, logs); got.PluginID != config.Plugin.Manifest.ID ||
				got.Level != pluginapi.LogInfo || got.Message != "device ready" {
				t.Fatalf("log entry = %#v", got)
			}

			process.wait <- nil
			assertUnexpectedExitResult(t, awaitSessionResult(t, session.Done()))
		})
	}
}

func TestPluginSessionReadyControlErrorsKeepTheirClassification(t *testing.T) {
	session, _, process := startReadyPluginSession(t, 23, sessionDependencies{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := session.Control(ctx, controlRequest{
		kind:  controlConfig,
		state: controlState{Config: pluginapi.Config{Revision: 2, Data: []byte(`{}`)}},
	})
	if !errors.Is(err, context.Canceled) || err.Error() != context.Canceled.Error() {
		t.Fatalf("Control(pre-canceled) error = %v, want unchanged context.Canceled", err)
	}
	err = session.Control(context.Background(), controlRequest{
		kind:  controlConfig,
		state: controlState{Config: pluginapi.Config{Revision: 2, Data: []byte(`{`)}},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid config") ||
		strings.Contains(err.Error(), "IPC writer failure") {
		t.Fatalf("Control(invalid config) error = %v, want unchanged validation error", err)
	}
	process.wait <- nil
	assertUnexpectedExitResult(t, awaitSessionResult(t, session.Done()))
}

func TestPluginSessionStopRequiresShutdownAckAndProcessExit(t *testing.T) {
	session, pluginConn, process := startReadyPluginSession(t, 9, sessionDependencies{})
	pluginDone := make(chan error, 1)
	go func() {
		message, err := pluginConn.Receive(context.Background())
		if err != nil {
			pluginDone <- err
			return
		}
		if _, ok := message.Payload.(protocol.Shutdown); !ok {
			pluginDone <- errors.New("host did not send Shutdown")
			return
		}
		ack, err := protocol.NewMessage(protocol.ShutdownAck{})
		if err == nil {
			err = pluginConn.Send(context.Background(), ack)
		}
		if err == nil {
			process.wait <- nil
		}
		pluginDone <- err
	}()

	ctx, cancel := context.WithTimeout(context.Background(), pluginSessionTestTimeout)
	defer cancel()
	if err := session.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if err := awaitValue(t, pluginDone); err != nil {
		t.Fatalf("plugin shutdown error = %v", err)
	}
	// Stop must wait on an internal completion signal, not consume the one
	// externally observable session result.
	if result := awaitSessionResult(t, session.Done()); result.Err != nil || result.StartedAt.IsZero() {
		t.Fatalf("Done() after Stop = %#v", result)
	}
}

type sessionBlockingConfigConn struct {
	protocol.Conn
	entered   chan struct{}
	release   chan struct{}
	closed    chan struct{}
	once      sync.Once
	closeOnce sync.Once
}

func (c *sessionBlockingConfigConn) Send(ctx context.Context, message protocol.Message) error {
	if message.Type == protocol.MessageConfigChanged {
		c.once.Do(func() { close(c.entered) })
		select {
		case <-c.release:
		case <-c.closed:
			return net.ErrClosed
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return c.Conn.Send(ctx, message)
}

func (c *sessionBlockingConfigConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

func TestPluginSessionAckBeforeShutdownSendDoesNotSatisfyStop(t *testing.T) {
	hostRaw, pluginRaw := net.Pipe()
	hostConn := &sessionBlockingConfigConn{
		Conn:    ipc.WrapConn(hostRaw),
		entered: make(chan struct{}),
		release: make(chan struct{}),
		closed:  make(chan struct{}),
	}
	pluginConn := ipc.WrapConn(pluginRaw)
	t.Cleanup(func() {
		_ = hostConn.Close()
		_ = pluginConn.Close()
	})
	process := newSessionTestProcess()
	token := handshakeToken(36)
	ready := make(chan struct{})
	session := newPluginSession(context.Background(), 28, sessionConfig{
		Plugin: InstalledPlugin{
			Manifest:   validManifest(),
			RootDir:    `C:\plugins\camera`,
			Executable: `C:\plugins\camera\plugin.exe`,
		},
		Startup:          validHandshakeStartup(),
		HandshakeTimeout: time.Second,
		HeartbeatTimeout: time.Minute,
		GracefulTimeout:  100 * time.Millisecond,
		KillTimeout:      50 * time.Millisecond,
		ControlCapacity:  3,
	}, sessionDependencies{
		credentials: func() (string, string, error) {
			return "session-early-ack", token, nil
		},
		listen: func(ipc.ServerConfig) (ipc.Listener, error) {
			return &sessionTestListener{conn: hostConn}, nil
		},
		launcher: sessionTestLauncher{start: func(context.Context, ProcessSpec) (Process, error) {
			go func() {
				_ = pluginConn.Send(context.Background(), validHello(token))
				_, _ = pluginConn.Receive(context.Background())
				message, _ := protocol.NewMessage(protocol.Ready{})
				if pluginConn.Send(context.Background(), message) == nil {
					close(ready)
				}
			}()
			return process, nil
		}},
	})
	awaitValue(t, ready)
	waitSessionPhase(t, session.(*processSession), sessionReady)
	controlResult := make(chan error, 1)
	go func() {
		controlResult <- session.Control(context.Background(), controlRequest{
			kind:  controlConfig,
			state: controlState{Config: pluginapi.Config{Revision: 2, Data: []byte(`{"value":2}`)}},
		})
	}()
	awaitValue(t, hostConn.entered)

	stopResult := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), pluginSessionTestTimeout)
		defer cancel()
		stopResult <- session.Stop(ctx)
	}()
	waitSessionPhase(t, session.(*processSession), sessionStopping)
	sendSessionPayload(t, pluginConn, protocol.ShutdownAck{})
	_ = awaitValue(t, controlResult)
	if err := awaitValue(t, stopResult); !errors.Is(err, ErrProtocolViolation) {
		t.Fatalf("Stop() error = %v, want ErrProtocolViolation for pre-send Ack", err)
	}
}

type sessionBlockingShutdownAttemptConn struct {
	protocol.Conn
	entered chan struct{}
	release chan struct{}
	sendErr error
	once    sync.Once
}

func (c *sessionBlockingShutdownAttemptConn) Send(ctx context.Context, message protocol.Message) error {
	if message.Type == protocol.MessageShutdown {
		c.once.Do(func() { close(c.entered) })
		select {
		case <-c.release:
		case <-ctx.Done():
			return ctx.Err()
		}
		if c.sendErr != nil {
			return c.sendErr
		}
	}
	return c.Conn.Send(ctx, message)
}

func TestPluginSessionAckDuringShutdownSendAttemptRequiresSendSuccess(t *testing.T) {
	for _, test := range []struct {
		name    string
		sendErr error
	}{
		{name: "successful send"},
		{name: "failed send", sendErr: errors.New("shutdown-send-secret")},
	} {
		t.Run(test.name, func(t *testing.T) {
			hostRaw, pluginRaw := net.Pipe()
			hostConn := &sessionBlockingShutdownAttemptConn{
				Conn:    ipc.WrapConn(hostRaw),
				entered: make(chan struct{}),
				release: make(chan struct{}),
				sendErr: test.sendErr,
			}
			pluginConn := ipc.WrapConn(pluginRaw)
			t.Cleanup(func() {
				_ = hostConn.Close()
				_ = pluginConn.Close()
			})
			process := newSessionTestProcess()
			token := handshakeToken(38)
			ready := make(chan struct{})
			session := newPluginSession(context.Background(), 30, sessionConfig{
				Plugin: InstalledPlugin{
					Manifest:   validManifest(),
					RootDir:    `C:\plugins\camera`,
					Executable: `C:\plugins\camera\plugin.exe`,
				},
				Startup:          validHandshakeStartup(),
				HandshakeTimeout: time.Second,
				HeartbeatTimeout: time.Minute,
				GracefulTimeout:  50 * time.Millisecond,
				KillTimeout:      50 * time.Millisecond,
				ControlCapacity:  2,
			}, sessionDependencies{
				credentials: func() (string, string, error) {
					return "session-ack-send-attempt", token, nil
				},
				listen: func(ipc.ServerConfig) (ipc.Listener, error) {
					return &sessionTestListener{conn: hostConn}, nil
				},
				launcher: sessionTestLauncher{start: func(context.Context, ProcessSpec) (Process, error) {
					go func() {
						_ = pluginConn.Send(context.Background(), validHello(token))
						_, _ = pluginConn.Receive(context.Background())
						message, _ := protocol.NewMessage(protocol.Ready{})
						if pluginConn.Send(context.Background(), message) == nil {
							close(ready)
						}
					}()
					return process, nil
				}},
			})
			awaitValue(t, ready)
			waitSessionPhase(t, session.(*processSession), sessionReady)
			stopResult := make(chan error, 1)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), pluginSessionTestTimeout)
				defer cancel()
				stopResult <- session.Stop(ctx)
			}()
			awaitValue(t, hostConn.entered)
			sendSessionPayload(t, pluginConn, protocol.ShutdownAck{})
			close(hostConn.release)

			if test.sendErr == nil {
				message, err := pluginConn.Receive(context.Background())
				if err != nil {
					t.Fatalf("receive Shutdown: %v", err)
				}
				if _, ok := message.Payload.(protocol.Shutdown); !ok {
					t.Fatalf("host payload = %T, want Shutdown", message.Payload)
				}
				process.wait <- nil
				if err := awaitValue(t, stopResult); err != nil {
					t.Fatalf("Stop() error = %v, want queued Ack to become effective", err)
				}
				return
			}

			err := awaitValue(t, stopResult)
			if !errors.Is(err, test.sendErr) || !errors.Is(err, ErrGracefulShutdownTimeout) {
				t.Fatalf("Stop() error = %v, want send failure and GracefulTimeout", err)
			}
			if strings.Contains(err.Error(), "shutdown-send-secret") {
				t.Fatalf("Stop() error exposes send failure: %v", err)
			}
			if got := process.kills(); got != 1 {
				t.Fatalf("Kill() calls = %d, want 1 after failed Shutdown send", got)
			}
		})
	}
}

func TestPluginSessionUnsolicitedZeroExitIsUnexpectedAndRetryable(t *testing.T) {
	session, _, process := startReadyPluginSession(t, 25, sessionDependencies{})
	process.wait <- nil
	result := awaitSessionResult(t, session.Done())
	if result.Err == nil {
		t.Fatal("unsolicited status-0 process exit produced a clean result")
	}
	if !result.Retryable {
		t.Fatalf("unsolicited status-0 result is not retryable: %v", result.Err)
	}
}

func TestPluginSessionGracefulTimeoutKillsOnce(t *testing.T) {
	session, pluginConn, process := startReadyPluginSession(t, 10, sessionDependencies{})
	go func() {
		_, _ = pluginConn.Receive(context.Background())
		// Deliberately omit ShutdownAck and leave the process running.
	}()

	ctx, cancel := context.WithTimeout(context.Background(), pluginSessionTestTimeout)
	defer cancel()
	err := session.Stop(ctx)
	if !errors.Is(err, ErrGracefulShutdownTimeout) {
		t.Fatalf("Stop() error = %v, want ErrGracefulShutdownTimeout", err)
	}
	if got := process.kills(); got != 1 {
		t.Fatalf("Kill() calls = %d, want 1", got)
	}
}

func TestPluginSessionKillTimeoutIsTerminal(t *testing.T) {
	session, pluginConn, process := startReadyPluginSession(t, 11, sessionDependencies{})
	process.mu.Lock()
	process.unblockOnKill = false
	process.mu.Unlock()
	go func() {
		_, _ = pluginConn.Receive(context.Background())
	}()

	ctx, cancel := context.WithTimeout(context.Background(), pluginSessionTestTimeout)
	defer cancel()
	err := session.Stop(ctx)
	if !errors.Is(err, ErrKillTimeout) {
		t.Fatalf("Stop() error = %v, want ErrKillTimeout", err)
	}
	if got := process.kills(); got != 1 {
		t.Fatalf("Kill() calls = %d, want 1", got)
	}
}

func TestPluginSessionHeartbeatTimeoutReportsUnresponsive(t *testing.T) {
	unresponsive := make(chan uint64, 1)
	session, _, process := startReadyPluginSessionWithHeartbeat(t, 12, sessionDependencies{
		onUnresponsive: func(instanceID uint64) {
			unresponsive <- instanceID
		},
	}, 30*time.Millisecond)
	if got := awaitValue(t, unresponsive); got != 12 {
		t.Fatalf("unresponsive instance ID = %d, want 12", got)
	}
	result := awaitSessionResult(t, session.Done())
	if !errors.Is(result.Err, ErrHeartbeatTimeout) {
		t.Fatalf("session error = %v, want ErrHeartbeatTimeout", result.Err)
	}
	if got := process.kills(); got != 1 {
		t.Fatalf("Kill() calls = %d, want 1", got)
	}
}

type sessionFailingReceiveConn struct {
	protocol.Conn
	mu       sync.Mutex
	receives int
	err      error
}

func (c *sessionFailingReceiveConn) Receive(ctx context.Context) (protocol.Message, error) {
	c.mu.Lock()
	c.receives++
	count := c.receives
	c.mu.Unlock()
	if count > 2 {
		return protocol.Message{}, c.err
	}
	return c.Conn.Receive(ctx)
}

type sessionFailingSendConn struct {
	protocol.Conn
	mu    sync.Mutex
	sends int
	err   error
}

func (c *sessionFailingSendConn) Send(ctx context.Context, message protocol.Message) error {
	c.mu.Lock()
	c.sends++
	count := c.sends
	c.mu.Unlock()
	if count > 1 {
		return c.err
	}
	return c.Conn.Send(ctx, message)
}

func TestPluginSessionWriterFailureTerminatesWithOpaqueCause(t *testing.T) {
	writerErr := errors.New("writer-secret-marker")
	hostRaw, pluginRaw := net.Pipe()
	hostConn := &sessionFailingSendConn{Conn: ipc.WrapConn(hostRaw), err: writerErr}
	pluginConn := ipc.WrapConn(pluginRaw)
	t.Cleanup(func() {
		_ = hostConn.Close()
		_ = pluginConn.Close()
	})
	process := newSessionTestProcess()
	token := handshakeToken(25)
	ready := make(chan struct{})
	session := newPluginSession(context.Background(), 15, sessionConfig{
		Plugin: InstalledPlugin{
			Manifest:   validManifest(),
			RootDir:    `C:\plugins\camera`,
			Executable: `C:\plugins\camera\plugin.exe`,
		},
		Startup:          validHandshakeStartup(),
		HandshakeTimeout: time.Second,
		HeartbeatTimeout: time.Minute,
		GracefulTimeout:  20 * time.Millisecond,
		KillTimeout:      20 * time.Millisecond,
		ControlCapacity:  2,
	}, sessionDependencies{
		credentials: func() (string, string, error) {
			return "session-writer-error", token, nil
		},
		listen: func(ipc.ServerConfig) (ipc.Listener, error) {
			return &sessionTestListener{conn: hostConn}, nil
		},
		launcher: sessionTestLauncher{start: func(context.Context, ProcessSpec) (Process, error) {
			go func() {
				_ = pluginConn.Send(context.Background(), validHello(token))
				_, _ = pluginConn.Receive(context.Background())
				message, _ := protocol.NewMessage(protocol.Ready{})
				if pluginConn.Send(context.Background(), message) == nil {
					close(ready)
				}
			}()
			return process, nil
		}},
	})
	awaitValue(t, ready)
	waitSessionPhase(t, session.(*processSession), sessionReady)
	controlErr := session.Control(context.Background(), controlRequest{
		kind:  controlConfig,
		state: controlState{Config: pluginapi.Config{Revision: 2, Data: []byte(`{"value":2}`)}},
	})
	if !errors.Is(controlErr, writerErr) {
		t.Fatalf("Control() error = %v, want writer cause", controlErr)
	}
	result := awaitSessionResult(t, session.Done())
	if !errors.Is(result.Err, writerErr) {
		t.Fatalf("session error = %v, want writer cause", result.Err)
	}
	if strings.Contains(result.Err.Error(), "writer-secret-marker") {
		t.Fatalf("session error exposes writer cause: %v", result.Err)
	}
}

func TestPluginSessionIPCEOFIsRetryableAndDiscoverable(t *testing.T) {
	session, pluginConn, _ := startReadyPluginSession(t, 18, sessionDependencies{})
	if err := pluginConn.Close(); err != nil {
		t.Fatal(err)
	}
	result := awaitSessionResult(t, session.Done())
	if !errors.Is(result.Err, io.EOF) && !errors.Is(result.Err, io.ErrClosedPipe) {
		t.Fatalf("session error = %v, want discoverable peer-close cause", result.Err)
	}
	if !result.Retryable {
		t.Fatal("IPC EOF result is not retryable")
	}
}

type sessionConcurrentFailureConn struct {
	protocol.Conn
	readerErr error
	writerErr error
	release   <-chan struct{}

	mu       sync.Mutex
	receives int
	sends    int
}

func (c *sessionConcurrentFailureConn) Receive(ctx context.Context) (protocol.Message, error) {
	c.mu.Lock()
	c.receives++
	count := c.receives
	c.mu.Unlock()
	if count > 2 {
		select {
		case <-c.release:
			return protocol.Message{}, c.readerErr
		default:
		}
		select {
		case <-c.release:
			return protocol.Message{}, c.readerErr
		case <-ctx.Done():
			return protocol.Message{}, ctx.Err()
		}
	}
	return c.Conn.Receive(ctx)
}

func (c *sessionConcurrentFailureConn) Send(ctx context.Context, message protocol.Message) error {
	c.mu.Lock()
	c.sends++
	count := c.sends
	c.mu.Unlock()
	if count > 1 {
		return c.writerErr
	}
	return c.Conn.Send(ctx, message)
}

func TestPluginSessionJoinsReaderWriterAndProcessErrorsWhenStopRaces(t *testing.T) {
	readerErr := errors.New("reader-concurrent-secret")
	writerErr := errors.New("writer-concurrent-secret")
	processErr := errors.New("process-concurrent-secret")
	releaseReader := make(chan struct{})
	hostRaw, pluginRaw := net.Pipe()
	hostConn := &sessionConcurrentFailureConn{
		Conn:      ipc.WrapConn(hostRaw),
		readerErr: readerErr,
		writerErr: writerErr,
		release:   releaseReader,
	}
	pluginConn := ipc.WrapConn(pluginRaw)
	t.Cleanup(func() {
		_ = hostConn.Close()
		_ = pluginConn.Close()
	})
	process := newSessionTestProcess()
	token := handshakeToken(28)
	ready := make(chan struct{})
	session := newPluginSession(context.Background(), 19, sessionConfig{
		Plugin: InstalledPlugin{
			Manifest:   validManifest(),
			RootDir:    `C:\plugins\camera`,
			Executable: `C:\plugins\camera\plugin.exe`,
		},
		Startup:          validHandshakeStartup(),
		HandshakeTimeout: time.Second,
		HeartbeatTimeout: time.Minute,
		GracefulTimeout:  30 * time.Millisecond,
		KillTimeout:      30 * time.Millisecond,
		ControlCapacity:  2,
	}, sessionDependencies{
		credentials: func() (string, string, error) {
			return "session-concurrent-errors", token, nil
		},
		listen: func(ipc.ServerConfig) (ipc.Listener, error) {
			return &sessionTestListener{conn: hostConn}, nil
		},
		launcher: sessionTestLauncher{start: func(context.Context, ProcessSpec) (Process, error) {
			go func() {
				_ = pluginConn.Send(context.Background(), validHello(token))
				_, _ = pluginConn.Receive(context.Background())
				message, _ := protocol.NewMessage(protocol.Ready{})
				if pluginConn.Send(context.Background(), message) == nil {
					close(ready)
				}
			}()
			return process, nil
		}},
	})
	awaitValue(t, ready)
	waitSessionPhase(t, session.(*processSession), sessionReady)
	if err := session.Control(context.Background(), controlRequest{
		kind:  controlConfig,
		state: controlState{Config: pluginapi.Config{Revision: 2, Data: []byte(`{"frameMarker":7654.25}`)}},
	}); !errors.Is(err, writerErr) {
		t.Fatalf("Control() error = %v, want writer cause", err)
	}
	process.wait <- processErr
	close(releaseReader)
	ctx, cancel := context.WithTimeout(context.Background(), pluginSessionTestTimeout)
	defer cancel()
	_ = session.Stop(ctx)
	result := awaitSessionResult(t, session.Done())
	for _, cause := range []error{readerErr, writerErr, processErr} {
		if !errors.Is(result.Err, cause) {
			t.Errorf("session error = %v, missing cause %v", result.Err, cause)
		}
	}
	for _, secret := range []string{
		"reader-concurrent-secret",
		"writer-concurrent-secret",
		"process-concurrent-secret",
		"7654.25",
	} {
		if strings.Contains(result.Err.Error(), secret) {
			t.Fatalf("session error exposes %q: %v", secret, result.Err)
		}
	}
}

type sessionErrorAfterCancelConn struct {
	protocol.Conn
	mu       sync.Mutex
	receives int
	err      error
}

func (c *sessionErrorAfterCancelConn) Receive(ctx context.Context) (protocol.Message, error) {
	c.mu.Lock()
	c.receives++
	count := c.receives
	c.mu.Unlock()
	if count > 2 {
		<-ctx.Done()
		return protocol.Message{}, c.err
	}
	return c.Conn.Receive(ctx)
}

func TestPluginSessionCancellationDoesNotReplacePrimaryReaderError(t *testing.T) {
	primaryErr := errors.New("reader-primary-secret")
	hostRaw, pluginRaw := net.Pipe()
	hostConn := &sessionErrorAfterCancelConn{Conn: ipc.WrapConn(hostRaw), err: primaryErr}
	pluginConn := ipc.WrapConn(pluginRaw)
	t.Cleanup(func() {
		_ = hostConn.Close()
		_ = pluginConn.Close()
	})
	process := newSessionTestProcess()
	token := handshakeToken(29)
	ready := make(chan struct{})
	session := newPluginSession(context.Background(), 20, sessionConfig{
		Plugin: InstalledPlugin{
			Manifest:   validManifest(),
			RootDir:    `C:\plugins\camera`,
			Executable: `C:\plugins\camera\plugin.exe`,
		},
		Startup:          validHandshakeStartup(),
		HandshakeTimeout: time.Second,
		HeartbeatTimeout: time.Minute,
		GracefulTimeout:  20 * time.Millisecond,
		KillTimeout:      20 * time.Millisecond,
		ControlCapacity:  2,
	}, sessionDependencies{
		credentials: func() (string, string, error) {
			return "session-cancel-primary", token, nil
		},
		listen: func(ipc.ServerConfig) (ipc.Listener, error) {
			return &sessionTestListener{conn: hostConn}, nil
		},
		launcher: sessionTestLauncher{start: func(context.Context, ProcessSpec) (Process, error) {
			go func() {
				_ = pluginConn.Send(context.Background(), validHello(token))
				_, _ = pluginConn.Receive(context.Background())
				message, _ := protocol.NewMessage(protocol.Ready{})
				if pluginConn.Send(context.Background(), message) == nil {
					close(ready)
				}
				_, _ = pluginConn.Receive(context.Background())
			}()
			return process, nil
		}},
	})
	awaitValue(t, ready)
	waitSessionPhase(t, session.(*processSession), sessionReady)
	ctx, cancel := context.WithTimeout(context.Background(), pluginSessionTestTimeout)
	defer cancel()
	_ = session.Stop(ctx)
	result := awaitSessionResult(t, session.Done())
	if !errors.Is(result.Err, primaryErr) {
		t.Fatalf("session error = %v, want primary reader cause", result.Err)
	}
	if strings.Contains(result.Err.Error(), "reader-primary-secret") {
		t.Fatalf("session error exposes primary reader cause: %v", result.Err)
	}
}

type sessionLateWriterFailureConn struct {
	protocol.Conn
	writerErr error
	entered   chan struct{}
	release   chan struct{}
	closed    chan struct{}

	mu        sync.Mutex
	sends     int
	closeOnce sync.Once
}

func (c *sessionLateWriterFailureConn) Send(ctx context.Context, message protocol.Message) error {
	c.mu.Lock()
	c.sends++
	count := c.sends
	c.mu.Unlock()
	if count > 1 {
		close(c.entered)
		select {
		case <-c.release:
			return c.writerErr
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return c.Conn.Send(ctx, message)
}

func (c *sessionLateWriterFailureConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

func TestPluginSessionProcessFirstDrainsLateWriterCause(t *testing.T) {
	writerErr := errors.New("late-writer-secret")
	processErr := errors.New("process-first-secret")
	hostRaw, pluginRaw := net.Pipe()
	hostConn := &sessionLateWriterFailureConn{
		Conn:      ipc.WrapConn(hostRaw),
		writerErr: writerErr,
		entered:   make(chan struct{}),
		release:   make(chan struct{}),
		closed:    make(chan struct{}),
	}
	pluginConn := ipc.WrapConn(pluginRaw)
	t.Cleanup(func() {
		_ = hostConn.Close()
		_ = pluginConn.Close()
	})
	process := newSessionTestProcess()
	token := handshakeToken(30)
	ready := make(chan struct{})
	session := newPluginSession(context.Background(), 21, sessionConfig{
		Plugin: InstalledPlugin{
			Manifest:   validManifest(),
			RootDir:    `C:\plugins\camera`,
			Executable: `C:\plugins\camera\plugin.exe`,
		},
		Startup:          validHandshakeStartup(),
		HandshakeTimeout: time.Second,
		HeartbeatTimeout: time.Minute,
		GracefulTimeout:  30 * time.Millisecond,
		KillTimeout:      30 * time.Millisecond,
		ControlCapacity:  2,
	}, sessionDependencies{
		credentials: func() (string, string, error) {
			return "session-process-first-writer", token, nil
		},
		listen: func(ipc.ServerConfig) (ipc.Listener, error) {
			return &sessionTestListener{conn: hostConn}, nil
		},
		launcher: sessionTestLauncher{start: func(context.Context, ProcessSpec) (Process, error) {
			go func() {
				_ = pluginConn.Send(context.Background(), validHello(token))
				_, _ = pluginConn.Receive(context.Background())
				message, _ := protocol.NewMessage(protocol.Ready{})
				if pluginConn.Send(context.Background(), message) == nil {
					close(ready)
				}
			}()
			return process, nil
		}},
	})
	awaitValue(t, ready)
	waitSessionPhase(t, session.(*processSession), sessionReady)
	controlResult := make(chan error, 1)
	go func() {
		controlResult <- session.Control(context.Background(), controlRequest{
			kind:  controlConfig,
			state: controlState{Config: pluginapi.Config{Revision: 2, Data: []byte(`{"value":2}`)}},
		})
	}()
	awaitValue(t, hostConn.entered)
	process.wait <- processErr
	awaitValue(t, hostConn.closed)
	close(hostConn.release)
	if err := awaitValue(t, controlResult); !errors.Is(err, writerErr) {
		t.Fatalf("Control() error = %v, want late writer cause", err)
	}
	result := awaitSessionResult(t, session.Done())
	if !errors.Is(result.Err, processErr) || !errors.Is(result.Err, writerErr) {
		t.Fatalf("session error = %v, want process and late writer causes", result.Err)
	}
	for _, secret := range []string{"late-writer-secret", "process-first-secret"} {
		if strings.Contains(result.Err.Error(), secret) {
			t.Fatalf("session error exposes %q: %v", secret, result.Err)
		}
	}
}

func waitSessionPhase(t *testing.T, session *processSession, want sessionPhase) {
	t.Helper()
	deadline := time.Now().Add(pluginSessionTestTimeout)
	for time.Now().Before(deadline) {
		session.mu.Lock()
		phase := session.phase
		session.mu.Unlock()
		if phase == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("session phase did not reach %d", want)
}

func TestPluginSessionRetainsConcurrentTerminalErrorsWithoutSecrets(t *testing.T) {
	readerErr := errors.New("reader-secret-marker")
	processErr := errors.New("process-secret-marker")
	configSecret := "config-secret-marker"
	hostRaw, pluginRaw := net.Pipe()
	hostConn := &sessionFailingReceiveConn{Conn: ipc.WrapConn(hostRaw), err: readerErr}
	pluginConn := ipc.WrapConn(pluginRaw)
	t.Cleanup(func() {
		_ = hostConn.Close()
		_ = pluginConn.Close()
	})
	process := newSessionTestProcess()
	token := handshakeToken(23)
	ready := make(chan struct{})
	dependencies := sessionDependencies{
		credentials: func() (string, string, error) {
			return "session-errors-pipe", token, nil
		},
		listen: func(ipc.ServerConfig) (ipc.Listener, error) {
			return &sessionTestListener{conn: hostConn}, nil
		},
		launcher: sessionTestLauncher{start: func(context.Context, ProcessSpec) (Process, error) {
			go func() {
				_ = pluginConn.Send(context.Background(), validHello(token))
				_, _ = pluginConn.Receive(context.Background())
				message, _ := protocol.NewMessage(protocol.Ready{})
				_ = pluginConn.Send(context.Background(), message)
				process.wait <- processErr
				close(ready)
			}()
			return process, nil
		}},
	}
	startup := validHandshakeStartup()
	startup.Config.Data = []byte(`{"value":"` + configSecret + `"}`)
	session := newPluginSession(context.Background(), 13, sessionConfig{
		Plugin: InstalledPlugin{
			Manifest:   validManifest(),
			RootDir:    `C:\plugins\camera`,
			Executable: `C:\plugins\camera\plugin.exe`,
		},
		Startup:          startup,
		HandshakeTimeout: time.Second,
		HeartbeatTimeout: time.Minute,
		GracefulTimeout:  20 * time.Millisecond,
		KillTimeout:      20 * time.Millisecond,
		ControlCapacity:  2,
	}, dependencies)
	awaitValue(t, ready)
	result := awaitSessionResult(t, session.Done())
	if !errors.Is(result.Err, readerErr) || !errors.Is(result.Err, processErr) {
		t.Fatalf("session error = %v, want both reader and process causes", result.Err)
	}
	for _, secret := range []string{token, configSecret, "reader-secret-marker", "process-secret-marker", "9876.5"} {
		if strings.Contains(result.Err.Error(), secret) {
			t.Fatalf("session error exposes %q: %v", secret, result.Err)
		}
	}
}

type sessionBlockingInitializeConn struct {
	protocol.Conn
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *sessionBlockingInitializeConn) Send(ctx context.Context, message protocol.Message) error {
	if message.Type == protocol.MessageInitialize {
		c.once.Do(func() { close(c.entered) })
		select {
		case <-c.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return c.Conn.Send(ctx, message)
}

type sessionReplayBlockingConn struct {
	protocol.Conn
	initializeEntered chan struct{}
	releaseInitialize chan struct{}
	replayEntered     chan struct{}
	initOnce          sync.Once
	replayOnce        sync.Once
}

type sessionGracefulReplayConn struct {
	protocol.Conn
	initializeEntered chan struct{}
	releaseInitialize chan struct{}
	replayEntered     chan struct{}
	releaseReplay     chan struct{}
	initOnce          sync.Once
	replayOnce        sync.Once
}

func (c *sessionGracefulReplayConn) Send(ctx context.Context, message protocol.Message) error {
	switch message.Type {
	case protocol.MessageInitialize:
		c.initOnce.Do(func() { close(c.initializeEntered) })
		select {
		case <-c.releaseInitialize:
		case <-ctx.Done():
			return ctx.Err()
		}
	case protocol.MessageConfigChanged:
		c.replayOnce.Do(func() { close(c.replayEntered) })
		select {
		case <-c.releaseReplay:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return c.Conn.Send(ctx, message)
}

func TestPluginSessionStopDuringReplayUsesGracefulShutdown(t *testing.T) {
	hostRaw, pluginRaw := net.Pipe()
	hostConn := &sessionGracefulReplayConn{
		Conn:              ipc.WrapConn(hostRaw),
		initializeEntered: make(chan struct{}),
		releaseInitialize: make(chan struct{}),
		replayEntered:     make(chan struct{}),
		releaseReplay:     make(chan struct{}),
	}
	pluginConn := ipc.WrapConn(pluginRaw)
	t.Cleanup(func() {
		_ = hostConn.Close()
		_ = pluginConn.Close()
	})
	process := newSessionTestProcess()
	token := handshakeToken(37)
	pluginDone := make(chan error, 1)
	session := newPluginSession(context.Background(), 29, sessionConfig{
		Plugin: InstalledPlugin{
			Manifest:   validManifest(),
			RootDir:    `C:\plugins\camera`,
			Executable: `C:\plugins\camera\plugin.exe`,
		},
		Startup:          validHandshakeStartup(),
		HandshakeTimeout: time.Second,
		HeartbeatTimeout: time.Minute,
		GracefulTimeout:  100 * time.Millisecond,
		KillTimeout:      50 * time.Millisecond,
		ControlCapacity:  2,
	}, sessionDependencies{
		credentials: func() (string, string, error) {
			return "session-replay-graceful", token, nil
		},
		listen: func(ipc.ServerConfig) (ipc.Listener, error) {
			return &sessionTestListener{conn: hostConn}, nil
		},
		launcher: sessionTestLauncher{start: func(context.Context, ProcessSpec) (Process, error) {
			go func() {
				if err := pluginConn.Send(context.Background(), validHello(token)); err != nil {
					pluginDone <- err
					return
				}
				if _, err := pluginConn.Receive(context.Background()); err != nil {
					pluginDone <- err
					return
				}
				ready, _ := protocol.NewMessage(protocol.Ready{})
				if err := pluginConn.Send(context.Background(), ready); err != nil {
					pluginDone <- err
					return
				}
				if _, err := pluginConn.Receive(context.Background()); err != nil {
					pluginDone <- err
					return
				}
				message, err := pluginConn.Receive(context.Background())
				if err != nil {
					pluginDone <- err
					return
				}
				if _, ok := message.Payload.(protocol.Shutdown); !ok {
					pluginDone <- errors.New("host did not send Shutdown after replay")
					return
				}
				ack, _ := protocol.NewMessage(protocol.ShutdownAck{})
				if err := pluginConn.Send(context.Background(), ack); err != nil {
					pluginDone <- err
					return
				}
				process.wait <- nil
				pluginDone <- nil
			}()
			return process, nil
		}},
	})
	awaitValue(t, hostConn.initializeEntered)
	controlResult := make(chan error, 1)
	go func() {
		controlResult <- session.Control(context.Background(), controlRequest{
			kind:  controlConfig,
			state: controlState{Config: pluginapi.Config{Revision: 2, Data: []byte(`{"gain":0.9}`)}},
		})
	}()
	waitSessionPendingControls(t, session.(*processSession), 1)
	close(hostConn.releaseInitialize)
	awaitValue(t, hostConn.replayEntered)

	stopResult := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), pluginSessionTestTimeout)
		defer cancel()
		stopResult <- session.Stop(ctx)
	}()
	waitSessionPhase(t, session.(*processSession), sessionStopping)
	close(hostConn.releaseReplay)
	if err := awaitValue(t, controlResult); err != nil {
		t.Fatalf("replayed Control() error = %v", err)
	}
	if err := awaitValue(t, pluginDone); err != nil {
		t.Fatalf("plugin graceful replay-stop error = %v", err)
	}
	if err := awaitValue(t, stopResult); err != nil {
		t.Fatalf("Stop() error = %v, want graceful nil", err)
	}
	if got := process.kills(); got != 0 {
		t.Fatalf("Kill() calls = %d, want 0 after graceful replay-stop", got)
	}
}

func (c *sessionReplayBlockingConn) Send(ctx context.Context, message protocol.Message) error {
	switch message.Type {
	case protocol.MessageInitialize:
		c.initOnce.Do(func() { close(c.initializeEntered) })
		select {
		case <-c.releaseInitialize:
		case <-ctx.Done():
			return ctx.Err()
		}
	case protocol.MessageConfigChanged:
		c.replayOnce.Do(func() { close(c.replayEntered) })
	}
	return c.Conn.Send(ctx, message)
}

func TestPluginSessionStopRemainsBoundedDuringBlockedHandshakeReplay(t *testing.T) {
	hostRaw, pluginRaw := net.Pipe()
	hostConn := &sessionReplayBlockingConn{
		Conn:              ipc.WrapConn(hostRaw),
		initializeEntered: make(chan struct{}),
		releaseInitialize: make(chan struct{}),
		replayEntered:     make(chan struct{}),
	}
	pluginConn := ipc.WrapConn(pluginRaw)
	t.Cleanup(func() {
		_ = hostConn.Close()
		_ = pluginConn.Close()
	})
	process := newSessionTestProcess()
	token := handshakeToken(32)
	session := newPluginSession(context.Background(), 24, sessionConfig{
		Plugin: InstalledPlugin{
			Manifest:   validManifest(),
			RootDir:    `C:\plugins\camera`,
			Executable: `C:\plugins\camera\plugin.exe`,
		},
		Startup:          validHandshakeStartup(),
		HandshakeTimeout: time.Second,
		HeartbeatTimeout: time.Minute,
		GracefulTimeout:  30 * time.Millisecond,
		KillTimeout:      30 * time.Millisecond,
		ControlCapacity:  2,
	}, sessionDependencies{
		credentials: func() (string, string, error) {
			return "session-replay-stop", token, nil
		},
		listen: func(ipc.ServerConfig) (ipc.Listener, error) {
			return &sessionTestListener{conn: hostConn}, nil
		},
		launcher: sessionTestLauncher{start: func(context.Context, ProcessSpec) (Process, error) {
			go func() {
				_ = pluginConn.Send(context.Background(), validHello(token))
				_, _ = pluginConn.Receive(context.Background())
				ready, _ := protocol.NewMessage(protocol.Ready{})
				_ = pluginConn.Send(context.Background(), ready)
				// The peer deliberately stops reading before replay.
			}()
			return process, nil
		}},
	})
	awaitValue(t, hostConn.initializeEntered)
	controlResult := make(chan error, 1)
	go func() {
		controlResult <- session.Control(context.Background(), controlRequest{
			kind:  controlConfig,
			state: controlState{Config: pluginapi.Config{Revision: 2, Data: []byte(`{"gain":0.8}`)}},
		})
	}()
	waitSessionPendingControls(t, session.(*processSession), 1)
	close(hostConn.releaseInitialize)
	awaitValue(t, hostConn.replayEntered)

	stopResult := make(chan error, 1)
	stopStarted := time.Now()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		defer cancel()
		stopResult <- session.Stop(ctx)
	}()
	var stopErr error
	select {
	case stopErr = <-stopResult:
	case <-time.After(500 * time.Millisecond):
		_ = pluginConn.Close()
		t.Fatal("Stop blocked behind handshake replay")
	}
	if !errors.Is(stopErr, ErrGracefulShutdownTimeout) {
		t.Fatalf("Stop() error = %v, want ErrGracefulShutdownTimeout", stopErr)
	}
	if elapsed := time.Since(stopStarted); elapsed < 20*time.Millisecond {
		t.Fatalf("Stop returned after %v, before GracefulTimeout", elapsed)
	}
	select {
	case <-controlResult:
	case <-time.After(pluginSessionTestTimeout):
		t.Fatal("accepted pending control was not completed during replay shutdown")
	}
	if got := process.kills(); got != 1 {
		t.Fatalf("Kill() calls = %d, want 1", got)
	}
}

func TestPluginSessionHandshakeControlRaceDeliversEveryChangeOnceInOrder(t *testing.T) {
	hostRaw, pluginRaw := net.Pipe()
	blockingHost := &sessionBlockingInitializeConn{
		Conn:    ipc.WrapConn(hostRaw),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	pluginConn := ipc.WrapConn(pluginRaw)
	t.Cleanup(func() {
		_ = blockingHost.Close()
		_ = pluginConn.Close()
	})
	process := newSessionTestProcess()
	token := handshakeToken(24)
	pluginMessages := make(chan []protocol.Message, 1)
	dependencies := sessionDependencies{
		credentials: func() (string, string, error) {
			return "session-control-race", token, nil
		},
		listen: func(ipc.ServerConfig) (ipc.Listener, error) {
			return &sessionTestListener{conn: blockingHost}, nil
		},
		launcher: sessionTestLauncher{start: func(context.Context, ProcessSpec) (Process, error) {
			go func() {
				_ = pluginConn.Send(context.Background(), validHello(token))
				initialize, err := pluginConn.Receive(context.Background())
				if err != nil {
					pluginMessages <- nil
					return
				}
				ready, _ := protocol.NewMessage(protocol.Ready{})
				if err := pluginConn.Send(context.Background(), ready); err != nil {
					pluginMessages <- nil
					return
				}
				messages := []protocol.Message{initialize}
				for range 3 {
					message, err := pluginConn.Receive(context.Background())
					if err != nil {
						pluginMessages <- nil
						return
					}
					messages = append(messages, message)
				}
				pluginMessages <- messages
			}()
			return process, nil
		}},
	}
	session := newPluginSession(context.Background(), 14, sessionConfig{
		Plugin: InstalledPlugin{
			Manifest:   validManifest(),
			RootDir:    `C:\plugins\camera`,
			Executable: `C:\plugins\camera\plugin.exe`,
		},
		Startup:          validHandshakeStartup(),
		HandshakeTimeout: time.Second,
		HeartbeatTimeout: time.Minute,
		GracefulTimeout:  50 * time.Millisecond,
		KillTimeout:      50 * time.Millisecond,
		ControlCapacity:  4,
	}, dependencies)
	awaitValue(t, blockingHost.entered)

	nextConfig := pluginapi.Config{Revision: 2, Data: []byte(`{"gain":0.75}`)}
	nextSubscription := pluginapi.Subscription{
		Generation:   1,
		Capabilities: trackingmodel.CapabilityEye,
	}
	controlErrors := make(chan error, 3)
	concrete := session.(*processSession)
	go func() {
		controlErrors <- session.Control(context.Background(), controlRequest{
			kind:  controlConfig,
			state: controlState{Config: nextConfig},
		})
	}()
	waitSessionPendingControls(t, concrete, 1)
	go func() {
		controlErrors <- session.Control(context.Background(), controlRequest{
			kind:  controlSubscription,
			state: controlState{Subscription: nextSubscription},
		})
	}()
	waitSessionPendingControls(t, concrete, 2)
	go func() {
		controlErrors <- session.Control(context.Background(), controlRequest{
			kind:  controlActive,
			state: controlState{Active: true},
		})
	}()
	waitSessionPendingControls(t, concrete, 3)
	close(blockingHost.release)

	messages := awaitValue(t, pluginMessages)
	if len(messages) != 4 {
		t.Fatalf("plugin observed %d messages, want Initialize plus 3 controls", len(messages))
	}
	initialize, ok := messages[0].Payload.(protocol.Initialize)
	if !ok {
		t.Fatalf("first payload = %T, want Initialize", messages[0].Payload)
	}
	if initialize.Startup.Config.Revision != 1 || initialize.Startup.Subscription.Generation != 0 || initialize.Startup.Active {
		t.Fatalf("Initialize unexpectedly contains racing controls: %#v", initialize.Startup)
	}
	if got, ok := messages[1].Payload.(protocol.ConfigChanged); !ok || !reflect.DeepEqual(got.Config, nextConfig) {
		t.Fatalf("second payload = %#v, want exact ConfigChanged", messages[1].Payload)
	}
	if got, ok := messages[2].Payload.(protocol.SubscriptionChanged); !ok || got.Subscription != nextSubscription {
		t.Fatalf("third payload = %#v, want exact SubscriptionChanged", messages[2].Payload)
	}
	if got, ok := messages[3].Payload.(protocol.ActiveChanged); !ok || !got.Active {
		t.Fatalf("fourth payload = %#v, want ActiveChanged(true)", messages[3].Payload)
	}
	for range 3 {
		if err := awaitValue(t, controlErrors); err != nil {
			t.Fatalf("Control() error = %v", err)
		}
	}

	process.wait <- nil
	assertUnexpectedExitResult(t, awaitSessionResult(t, session.Done()))
}

func assertUnexpectedExitResult(t *testing.T, result sessionResult) {
	t.Helper()
	if result.Err == nil || !result.Retryable {
		t.Fatalf("unsolicited process exit result = %#v, want retryable error", result)
	}
}

func waitSessionPendingControls(t *testing.T, session *processSession, want int) {
	t.Helper()
	deadline := time.Now().Add(pluginSessionTestTimeout)
	for time.Now().Before(deadline) {
		session.mu.Lock()
		count := len(session.pending)
		session.mu.Unlock()
		if count == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("pending control count did not reach %d", want)
}

func startReadyPluginSession(
	t *testing.T,
	instanceID uint64,
	overrides sessionDependencies,
) (pluginSession, protocol.Conn, *sessionTestProcess) {
	return startReadyPluginSessionWithHeartbeat(t, instanceID, overrides, time.Minute)
}

func startReadyPluginSessionWithHeartbeat(
	t *testing.T,
	instanceID uint64,
	overrides sessionDependencies,
	heartbeatTimeout time.Duration,
) (pluginSession, protocol.Conn, *sessionTestProcess) {
	t.Helper()
	hostRaw, pluginRaw := net.Pipe()
	hostConn := ipc.WrapConn(hostRaw)
	pluginConn := ipc.WrapConn(pluginRaw)
	t.Cleanup(func() {
		_ = hostConn.Close()
		_ = pluginConn.Close()
	})
	process := newSessionTestProcess()
	token := handshakeToken(byte(instanceID + 20))
	ready := make(chan struct{})
	dependencies := overrides
	dependencies.credentials = func() (string, string, error) {
		return "session-shutdown-pipe", token, nil
	}
	dependencies.listen = func(ipc.ServerConfig) (ipc.Listener, error) {
		return &sessionTestListener{conn: hostConn}, nil
	}
	dependencies.launcher = sessionTestLauncher{start: func(context.Context, ProcessSpec) (Process, error) {
		go func() {
			_ = pluginConn.Send(context.Background(), validHello(token))
			_, _ = pluginConn.Receive(context.Background())
			message, _ := protocol.NewMessage(protocol.Ready{})
			if pluginConn.Send(context.Background(), message) == nil {
				close(ready)
			}
		}()
		return process, nil
	}}
	session := newPluginSession(context.Background(), instanceID, sessionConfig{
		Plugin: InstalledPlugin{
			Manifest:   validManifest(),
			RootDir:    `C:\plugins\camera`,
			Executable: `C:\plugins\camera\plugin.exe`,
		},
		Startup:          validHandshakeStartup(),
		HandshakeTimeout: time.Second,
		HeartbeatTimeout: heartbeatTimeout,
		GracefulTimeout:  30 * time.Millisecond,
		KillTimeout:      30 * time.Millisecond,
		ControlCapacity:  4,
	}, dependencies)
	awaitValue(t, ready)
	waitSessionPhase(t, session.(*processSession), sessionReady)
	return session, pluginConn, process
}

func sendSessionPayload(t *testing.T, conn protocol.Conn, payload any) {
	t.Helper()
	message, err := protocol.NewMessage(payload)
	if err != nil {
		t.Fatalf("protocol.NewMessage(%T) error = %v", payload, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), pluginSessionTestTimeout)
	defer cancel()
	if err := conn.Send(ctx, message); err != nil {
		t.Fatalf("Send(%T) error = %v", payload, err)
	}
}

func awaitValue[T any](t *testing.T, values <-chan T) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(pluginSessionTestTimeout):
		t.Fatal("timed out waiting for session observation")
		var zero T
		return zero
	}
}

func awaitSessionResult(t *testing.T, done <-chan sessionResult) sessionResult {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(pluginSessionTestTimeout):
		t.Fatal("plugin session did not finish")
		return sessionResult{}
	}
}
