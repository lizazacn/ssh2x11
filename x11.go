package ssh2x11

import (
	"encoding/hex"
	"errors"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"runtime"
	"sync"
)

// X11Request X11请求类型
type X11Request struct {
	SingleConnection bool
	AuthProtocol     string // 校验协议
	AuthCookie       string // 验证Cookie
	ScreenNumber     uint32 // 屏幕编号
}

func NewX11Session(client *ssh.Client, conn interface{}) (*ssh.Session, chan error) {
	var x11Request = new(X11Request)
	x11Request.AuthProtocol = AUTH_MIT_MAGIC_COOKIE_1
	x11Request.AuthCookie = newCookie(16)
	x11Request.ScreenNumber = 0
	x11Request.SingleConnection = false
	return CreateX11Session(client, x11Request, conn)
}

func CreateX11Session(client *ssh.Client, x11Request *X11Request, conn interface{}) (*ssh.Session, chan error) {
	var errChan = make(chan error, 10)
	session, err := client.NewSession()
	if err != nil {
		errChan <- err
		return nil, errChan
	}
	if conn == nil {
		if runtime.GOOS == "windows" {
			conn, err = net.Dial("tcp", "127.0.0.1:6000")
			if err != nil {
				errChan <- err
				return nil, errChan
			}
		} else {
			conn, err = net.Dial("unix", "/tmp/.X11-unix/X0")
			if err != nil {
				log.Printf("建立unix连接异常，尝试建立TCP连接！\n")
				conn, err = net.Dial("tcp", "127.0.0.1:6000")
				if err != nil {
					errChan <- err
					return nil, errChan
				}
			}
		}
	}
	_, err = session.SendRequest("x11-req", false, ssh.Marshal(x11Request))
	if err != nil {
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

func forwardToLocal(channel ssh.Channel, conn interface{}) error {
	if channel == nil {
		return errors.New("sshChannel is not exits")
	}
	var errChan = make(chan error, 4)
	switch conn.(type) {
	case net.Conn:
		NetConnForward(conn.(net.Conn), channel, errChan)
	case *websocket.Conn:
		WsConnForward(conn.(*websocket.Conn), channel, errChan)
	case *os.File:
		FileForward(conn.(*os.File), channel, errChan)
	default:
		return errors.New("conn类型不在许可范围！")
	}
	select {
	case err := <-errChan:
		return err
	default:
		return nil
	}
}

// NetConnForward 转发到NetConn
func NetConnForward(conn net.Conn, channel ssh.Channel, errChan chan error) {
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
}

// WsConnForward 转发到WSConn
func WsConnForward(conn *websocket.Conn, channel ssh.Channel, errChan chan error) {
	_, reader, err := conn.NextReader()
	if err != nil {
		return
	}
	writer, err := conn.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return
	}
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer func(conn *websocket.Conn) {
			errChan <- conn.Close()
		}(conn)
		defer wait.Done()
		_, err = io.Copy(writer, channel)
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
		_, err = io.Copy(channel, reader)
		if err != nil {
			errChan <- err
			return
		}
	}()
	wait.Wait()
}

// FileForward 转发到文件
func FileForward(file *os.File, channel ssh.Channel, errChan chan error) {

	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer func(file *os.File) {
			errChan <- file.Close()
		}(file)
		defer wait.Done()
		_, err := io.Copy(file, channel)
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
		_, err := io.Copy(channel, file)
		if err != nil {
			errChan <- err
			return
		}
	}()
	wait.Wait()
}
