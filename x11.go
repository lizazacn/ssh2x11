package ssh2x11

import (
	"encoding/hex"
	"errors"
	"golang.org/x/crypto/ssh"
	"io"
	"math/rand"
	"net"
	"sync"
)

// X11Request X11请求类型
type X11Request struct {
	SingleConnection bool
	AuthProtocol     string // 校验协议
	AuthCookie       string // 验证Cookie
	ScreenNumber     uint32 // 屏幕编号
}

func NewX11Session(client *ssh.Client, conn net.Conn) (*ssh.Session, chan error) {
	var x11Request = new(X11Request)
	x11Request.AuthProtocol = AUTH_MIT_MAGIC_COOKIE_1
	x11Request.AuthCookie = newCookie(16)
	x11Request.ScreenNumber = 0
	x11Request.SingleConnection = false
	return CreateX11Session(client, x11Request, conn)
}

func CreateX11Session(client *ssh.Client, x11Request *X11Request, conn net.Conn) (*ssh.Session, chan error) {
	var errChan = make(chan error, 10)
	session, err := client.NewSession()
	if err != nil {
		errChan <- err
		return nil, errChan
	}
	request, err := session.SendRequest("x11-req", false, ssh.Marshal(x11Request))
	if err != nil || !request {
		errChan <- err
		return nil, errChan
	}
	channel := client.HandleChannelOpen("x11")
	go func() {
		for c := range channel {
			ch, _, err := c.Accept()
			if err != nil {
				errChan <- err
				return
			}
			go func() {
				err := forwardToLocal(ch, conn)
				if err != nil {
					errChan <- err
					return
				}
			}()
		}
	}()
	return session, errChan
}

func newCookie(length int) string {
	randomBytes := make([]byte, length)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return ""
	}
	return hex.EncodeToString(randomBytes)
}

func forwardToLocal(channel ssh.Channel, conn net.Conn) error {
	if conn == nil || channel == nil {
		return errors.New("netConn or sshChannel is not exits")
	}
	var errChan = make(chan error, 4)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer func(conn net.Conn) {
			errChan <- conn.Close()
		}(conn)
		defer wait.Done()
		_, err := io.Copy(conn, channel)
		if err != nil {
			errChan <- err
			return
		}
	}()

	go func() {
		defer func(channel ssh.Channel) {
			errChan <- channel.CloseWrite()

		}(channel)
		defer wait.Done()
		_, err := io.Copy(channel, conn)
		if err != nil {
			errChan <- err
			return
		}
	}()
	wait.Wait()
	select {
	case err := <-errChan:
		return err
	default:
		return nil
	}
}
