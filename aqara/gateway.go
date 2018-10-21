package aqara

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"encoding/json"
	"net"
	"sync"
)

// https://aqara.gitbooks.io/lumi-gateway-lan-communication-api/content/

// Gateway represent a Xiaomi Aqara Gateway
type Gateway struct {
	IP     net.IP
	Port   int
	Sid    string
	Key    string
	Iface  string
	token  string
	socket *net.UDPConn
	lock   *sync.Mutex
}

// NewGateway create a Xiaomi Gateway instance
func NewGateway(ip net.IP, sid string, key string, iface string) Gateway {
	return Gateway{
		IP:    ip,
		Port:  9898,
		Sid:   sid,
		Key:   key,
		Iface: iface,
		lock:  &sync.Mutex{},
	}
}

type deviceMessage struct {
	Token string `json:"token"`
	Data  string `json:"data"`
}

// GetDevices return a list of SID registered
func (g *Gateway) GetDevices() ([]string, error) {
	resp, err := g.communicate([]byte("{\"cmd\":\"get_id_list\"}"))
	if err != nil {
		return nil, err
	}

	var message deviceMessage
	if err = json.Unmarshal(resp, &message); err != nil {
		return nil, err
	}
	g.token = message.Token

	var devices []string
	if err = json.Unmarshal([]byte(message.Data), &devices); err != nil {
		return nil, err
	}

	return devices, nil
}

type deviceStatusMessage struct {
	Cmd string `json:"cmd"`
	Sid string `json:"sid"`
}

// GetDeviceStatus query a device's status from gateway
func (g *Gateway) GetDeviceStatus(sid string) (*ReportMessage, error) {
	message, err := json.Marshal(deviceStatusMessage{
		Cmd: "read",
		Sid: sid,
	})
	if err != nil {
		return nil, err
	}

	resp, err := g.communicate(message)
	if err != nil {
		return nil, err
	}

	return parseBuffer(resp, g)
}

type setMesage struct {
	Cmd  string `json:"cmd"`
	Sid  string `json:"sid"`
	Data string `json:"data"`
}

type setGatewayMessage struct {
	RGB int64  `json:"rgb"`
	Key string `json:"key,omitempty"`
}

// SetRGB set the gateway's light
func (g *Gateway) SetRGB(rgb int64) (*ReportMessage, error) {
	inner, err := json.Marshal(setGatewayMessage{
		RGB: rgb,
		Key: g.getKey(),
	})
	if err != nil {
		return nil, err
	}

	message, err := json.Marshal(setMesage{
		Cmd:  "write",
		Sid:  g.Sid,
		Data: string(inner),
	})
	if err != nil {
		return nil, err
	}

	resp, err := g.communicate(message)
	if err != nil {
		return nil, err
	}

	return parseBuffer(resp, g)
}

// send one message and receive one response
func (g *Gateway) communicate(message []byte) ([]byte, error) {
	if err := g.checkConnection(); err != nil {
		return nil, err
	}

	g.lock.Lock()
	defer g.lock.Unlock()

	if _, err := g.socket.Write(message); err != nil {
		return nil, err
	}

	buffer := make([]byte, 1024)
	size, err := g.socket.Read(buffer)
	if err != nil {
		return nil, err
	}

	return buffer[:size], nil
}

// check whether connection exists, or create them if it isn't
func (g *Gateway) checkConnection() error {
	g.lock.Lock()
	defer g.lock.Unlock()

	if g.socket != nil {
		return nil
	}

	addr, err := getInterfaceAddr(g.Iface)
	if err != nil {
		return err
	}

	con, err := net.DialUDP("udp", &net.UDPAddr{IP: addr, Port: 0}, &net.UDPAddr{
		IP:   g.IP,
		Port: g.Port,
	})
	if err != nil {
		return err
	}

	g.socket = con
	return nil
}

var iv = []byte{0x17, 0x99, 0x6d, 0x09, 0x3d, 0x28, 0xdd, 0xb3, 0xba, 0x69, 0x5a, 0x2e, 0x6f, 0x58, 0x56, 0x2e}

func (g *Gateway) getKey() string {
	if g.Key == "" || g.token == "" {
		return ""
	}
	block, err := aes.NewCipher([]byte(g.Key))
	if err != nil {
		return ""
	}

	encrypter := cipher.NewCBCEncrypter(block, iv)
	out := make([]byte, aes.BlockSize)
	encrypter.CryptBlocks(out, []byte(g.token))
	return hex.EncodeToString(out)
}