package sshutil

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
)

func testSSHClient(t *testing.T, client *ssh.Client) {
	t.Run("session_exec", func(t *testing.T) { testSessionExec(t, client) })
	t.Run("session_exec_stderr", func(t *testing.T) { testSessionPipes(t, client) })
	t.Run("session_stdin", func(t *testing.T) { testStdin(t, client) })
	t.Run("session_exit_code", func(t *testing.T) { testExitCode(t, client) })
	t.Run("environment_variables", func(t *testing.T) { testEnvironmentVar(t, client) })
	t.Run("tcp_forward_local", func(t *testing.T) { testTCPLocal(t, client) })
	t.Run("tcp_forward_remote", func(t *testing.T) { testTCPRemote(t, client) })
	t.Run("unix_forward", func(t *testing.T) { t.Skip(); testUnixForward(t, client) })
	t.Run("invalid_request", func(t *testing.T) { testRequestError(t, client) })
	t.Run("channel_error", func(t *testing.T) { testChannelError(t, client) })
	t.Run("x11_request", func(t *testing.T) { testX11Forwarding(t, client) })
}

func testTCPLocal(t *testing.T, client *ssh.Client) {
	left, right, err := tcpPipeWithDialer(client.Dial, net.Listen)
	if err != nil {
		t.Fatalf("new net pipe: %v", err)
	}
	testConnPipe(t, left, right)
}

func testConnPipe(t *testing.T, a, b net.Conn) {
	mockdata := strconv.Itoa(rand.Int()) + "\n"

	_, err := a.Write([]byte(mockdata))
	if err != nil {
		t.Fatalf("write mock data: %v", err)
	}

	err = a.Close()
	if err != nil {
		t.Fatalf("close conn: %v", err)
	}

	content, err := bufio.NewReader(b).ReadString('\n')
	if err != nil {
		t.Fatalf("read from net conn: %v", err)
	}
	if content != mockdata {
		t.Fatalf("unexpected data, expected (%s), got (%s)", mockdata, content)
	}
}

func testUnixForward(t *testing.T, client *ssh.Client) {
	left, right, err := unixSocketPipe(t, client.Dial, net.Listen)
	if err != nil {
		t.Fatalf("new unix socket pipe: %v", err)
	}
	testConnPipe(t, left, right)
}

func testX11Forwarding(t *testing.T, client *ssh.Client) {
	_, _, err := client.SendRequest("x11-req", true, nil)
	if err != nil {
		t.Fatalf("new x11 forward request: %v", err)
	}
}

func testTCPRemote(t *testing.T, client *ssh.Client) {
	left, right, err := tcpPipeWithDialer(net.Dial, client.Listen)
	if err != nil {
		t.Fatalf("new remote tcp conn pipe: %v", err)
	}
	testConnPipe(t, left, right)
}

func testSessionExec(t *testing.T, client *ssh.Client) {
	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("new ssh session: %v", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput("echo 123")
	if err != nil {
		t.Fatalf("execute command: %v", err)
	}
	if string(output) != "123\n" {
		t.Fatalf("unexpected output, expected (%s), got (%s)", "123\n", string(output))
	}

	session, err = client.NewSession()
	if err != nil {
		t.Fatalf("new ssh session: %v", err)
	}
	defer session.Close()

	// create a pipe to simulate a hanging stdin
	// never write or close the write end
	r, _ := io.Pipe()
	session.Stdin = r

	output, err = session.CombinedOutput("echo 123")
	if err != nil {
		t.Fatalf("new ssh session: %v", err)
	}
	if string(output) != "123\n" {
		t.Fatalf("unexpected command output, expected (%s), got (%s)", "123\n", string(output))
	}
}

func testSessionPipes(t *testing.T, client *ssh.Client) {
	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("new ssh session: %v", err)
	}
	defer session.Close()

	stderrPipe, err := session.StderrPipe()
	if err != nil {
		t.Fatalf("new stderr pipe: %v", err)
	}

	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		t.Fatalf("new stdout pipe: %v", err)
	}

	var wg sync.WaitGroup
	stderr, stdout := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(stderr, stderrPipe)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(stdout, stdoutPipe)
	}()

	err = session.Run(">&2 echo error")
	if err != nil {
		t.Fatalf("run command: %v", err)
	}

	wg.Wait()
	session.Close()
}

func testStdin(t *testing.T, client *ssh.Client) {
	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("new ssh session: %v", err)
	}
	defer session.Close()

	session.Stdin = strings.NewReader("testing\n")

	output, err := session.CombinedOutput("cat")
	if err != nil {
		t.Fatalf("execute command: %v", err)
	}
	if string(output) != "testing\n" {
		t.Fatalf("unexpected output, expected (%s), got (%s)", "testing", string(output))
	}
}

func testExitCode(t *testing.T, client *ssh.Client) {
	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("new ssh session: %v", err)
	}
	defer session.Close()

	err = session.Run("exit 123")
	if err == nil {
		t.Fatalf("expected error, got: %v", err)
	}

	var exitErr *ssh.ExitError
	ok := errors.As(err, &exitErr)
	if !ok {
		t.Fatalf("unknown error type, expected ssh.ExitError: %v", err)
	}
	if exitErr.ExitStatus() != 123 {
		t.Fatalf("unexpected exit status, expected %d, got %d", 123, exitErr.ExitStatus())
	}
}

func testEnvironmentVar(t *testing.T, client *ssh.Client) {
	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("new ssh session: %v", err)
	}
	defer session.Close()

	setEnvs := map[string]string{
		"NEW_ENV": "TEST_VALUE",
		"TESTING": "with space",
	}

	for k, v := range setEnvs {
		err := session.Setenv(k, v)
		if err != nil {
			t.Fatalf("set enviornment variable: %v", err)
		}
	}

	output, err := session.CombinedOutput("env")
	if err != nil {
		t.Fatalf("run comamnd: %v", err)
	}

	env := string(output)
	for k, v := range setEnvs {
		contains := strings.Contains(env, fmt.Sprintf("%s=%s", k, v))
		if !contains {
			t.Fatalf("environment var not found: %v", err)
		}
	}
}

func testChannelError(t *testing.T, client *ssh.Client) {
	var openChErr *ssh.OpenChannelError
	_, _, err := client.OpenChannel("invalid", []byte{})
	if err == nil {
		t.Fatalf("expected error from open invalid channel, got %v", err)
	}
	if !errors.As(err, &openChErr) {
		t.Fatalf("expected *ssh.OpenChannelError, got %T: %v", err, err)
	}
	if openChErr.Reason != ssh.ConnectionFailed {
		t.Fatalf("expected ssh.ConnectionFailed, got: %s", openChErr.Reason.String())
	}
}

func testRequestError(t *testing.T, client *ssh.Client) {
	ok, resp, err := client.SendRequest("invalid", true, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp) != 0 {
		t.Fatalf("expected request response to be empty, got %v", resp)
	}
	if ok {
		t.Fatalf("expected false from \"ok\"")
	}
}

type listener func(net string, addr string) (net.Listener, error)
type dialer func(net, addr string) (net.Conn, error)

func tcpPipeWithDialer(dial dialer, listen listener) (net.Conn, net.Conn, error) {
	// may need to use "[::1]:0" for ipv6
	return netPipeWithDialer(dial, listen, "tcp", "127.0.0.1:0")
}

func unixSocketPipe(t *testing.T, dial dialer, listen listener) (net.Conn, net.Conn, error) {
	socket := filepath.Join("/tmp", "sshutil-unix-test-"+strconv.Itoa(rand.Int())+".sock")
	cleanup := func() { _ = os.Remove(socket) }
	cleanup()
	t.Cleanup(cleanup)
	return netPipeWithDialer(dial, listen, "unix", socket)
}

func netPipeWithDialer(dial dialer, listen listener, net, addr string) (net.Conn, net.Conn, error) {
	listener, err := listen(net, addr)
	if err != nil {
		return nil, nil, err
	}
	defer listener.Close()

	c1, err := dial(net, listener.Addr().String())
	if err != nil {
		return nil, nil, err
	}

	c2, err := listener.Accept()
	if err != nil {
		c1.Close()
		return nil, nil, err
	}

	return c1, c2, nil
}
