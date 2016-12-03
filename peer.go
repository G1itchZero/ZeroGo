package main

import (
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"

	_ "github.com/Sirupsen/logrus"
	msgpack "gopkg.in/vmihailenco/msgpack.v2"
)

type State int

const (
	Disconnected State = iota
	Connecting
	Connected
)

type Handshake struct {
	Version        string `msgpack:"version"`
	Rev            int    `msgpack:"rev"`
	Protocol       string `msgpack:"protocol"`
	PeerID         string `msgpack:"peer_id"`
	FileserverPort int    `msgpack:"fileserver_port"`
	PortOpened     bool   `msgpack:"port_opened"`
	TargetIP       string `msgpack:"target_ip"`
	CryptSupported bool   `msgpack:"crypt_supported"`
	Crypt          string `msgpack:"crypt"`
}

type RequestFile struct {
	Site      string `msgpack:"site"`
	InnerPath string `msgpack:"inner_path"`
	Location  int    `msgpack:"location"`
}

type Request struct {
	Cmd    string      `msgpack:"cmd"`
	ReqID  int         `msgpack:"req_id"`
	Params interface{} `msgpack:"params"`
}

type Response struct {
	Cmd         string `msgpack:"cmd"`
	ReqID       int    `msgpack:"req_id"`
	Body        string `msgpack:"body"`
	Size        int    `msgpack:"size"`
	StreamBytes int    `msgpack:"stream_bytes"`
	To          int    `msgpack:"to"`
	Location    int    `msgpack:"location"`
	Buffer      []byte
}

type Peer struct {
	State      State
	Address    string
	Port       uint64
	Connection *tls.Conn
	ReqID      int
}

func (peer *Peer) send(request Request) Response {
	request.ReqID = peer.ReqID
	// log.WithFields(log.Fields{"request": request}).Info("Sending")
	data, _ := msgpack.Marshal(request)
	_, _ = peer.Connection.Write(data)
	peer.ReqID++

	message := make([]byte, 1024*16)
	_, _ = peer.Connection.Read(message)
	answer := Response{}
	msgpack.Unmarshal(message, &answer)
	// log.WithFields(log.Fields{"response": answer}).Info("Recv")
	if answer.StreamBytes > 0 {
		answer.Buffer = []byte{}
		left := answer.StreamBytes
		for left > 0 {
			n, _ := peer.Connection.Read(message)
			left = left - n
			answer.Buffer = append(answer.Buffer, message...)
		}
		answer.Buffer = answer.Buffer[0:answer.StreamBytes]
	}
	return answer
}

func (peer *Peer) Download(site string, inner_path string) []byte {
	filename := path.Join(DATA, site, inner_path)
	os.MkdirAll(path.Dir(filename), 0777)
	location := 0
	request := Request{
		Cmd: "streamFile",
		Params: RequestFile{
			Site:      site,
			InnerPath: inner_path,
			Location:  location,
		},
	}
	message := peer.send(request)
	ioutil.WriteFile(filename, message.Buffer, 0644)
	return message.Buffer
}

func (peer *Peer) Ping() {
	ping := Request{
		Cmd:    "ping",
		Params: map[string]string{},
	}
	pong := peer.send(ping)
	if pong.Body == "Pong!" {
		// fmt.Println("Ping successfull")
	}
}

func (peer *Peer) Handshake() Response {
	hs := Request{
		Cmd: "handshake",
		Params: Handshake{
			Version:        VERSION,
			Rev:            REV,
			Protocol:       "v2",
			PeerID:         PEER_ID,
			FileserverPort: 0,
			PortOpened:     false,
			TargetIP:       peer.Address,
			CryptSupported: true,
			Crypt:          "tls-rsa",
		},
	}
	return peer.send(hs)
}

func (peer *Peer) Connect() error {
	peer.State = Connecting
	certFilename := path.Join(DATA, "cert-rsa.pem")
	keyFilename := path.Join(DATA, "key-rsa.pem")
	cert, err := tls.LoadX509KeyPair(certFilename, keyFilename)
	conn, err := tls.Dial("tcp", fmt.Sprintf("%s:%d",
		peer.Address,
		peer.Port), &tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{cert},
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		},
	})
	if err == nil {
		peer.Connection = conn
		peer.State = Connected
		peer.Handshake()
	}
	return err
}

func NewPeer(info io.Reader) *Peer {
	addr := [4]byte{}
	port := [2]byte{}
	binary.Read(info, binary.BigEndian, &addr)
	binary.Read(info, binary.BigEndian, &port)
	peer := Peer{State: Disconnected}
	peer.Address = fmt.Sprintf("%d.%d.%d.%d", addr[0], addr[1], addr[2], addr[3])
	peer.Port = binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, port[0], port[1]})
	return &peer
}
