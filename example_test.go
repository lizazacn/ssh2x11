package ssh2x11

import (
	"fmt"
	"golang.org/x/crypto/ssh"
	"testing"
)

func TestSSH(t *testing.T) {
	config := &ssh.ClientConfig{
		User: "wangnana",
		Auth: []ssh.AuthMethod{
			ssh.Password("wangnana?"),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	// 连接 SSH 服务器
	client, err := ssh.Dial("tcp", "localhost:22", config)
	if err != nil {
		fmt.Println("Failed to connect:", err)
		return
	}
	defer client.Close()

	session, _ := NewX11Session(client, nil)
	err = session.Run("xclock")
	if err != nil {
		return
	}

	err = session.Shell()
	if err != nil {
		return
	}
	err = session.Wait()
	if err != nil {
		return
	}
}
