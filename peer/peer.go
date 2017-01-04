package peer

import (
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	r "math/rand"
	"net"
	"os"
	"path"
	"sync"
	"time"

	"github.com/G1itchZero/ZeroGo/interfaces"
	"github.com/G1itchZero/ZeroGo/utils"
	_ "github.com/Sirupsen/logrus"
	log "github.com/Sirupsen/logrus"
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
	Size   int64
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
	State       State
	Address     string
	Port        uint64
	Connection  *tls.Conn
	ReqID       int
	Tasks       []interfaces.ITask
	ActiveTasks int
	Cancel      chan struct{}
	buffers     map[int][]byte
	chans       map[int]chan Response
	Ticker      *time.Ticker
	Listening   bool
	Free        chan *Peer
	wlock       *sync.Mutex
	sizes       map[int]int64
	sync.Mutex
}

func (peer *Peer) AddTask(task interfaces.ITask) error {
	peer.Tasks = append(peer.Tasks, task)
	_, res := peer.Download(task)
	peer.Free <- peer
	return res
}

func (peer *Peer) RemoveTask(task interfaces.ITask) {
	i := func() int {
		i := 0
		for _, b := range peer.Tasks {
			if b.GetSite() == task.GetSite() {
				return i
			}
			i++
		}
		return -1
	}()
	if i == -1 {
		return
	}
	peer.Tasks = append(peer.Tasks[:i], peer.Tasks[i+1:]...)
}

func (peer *Peer) String() string {
	return fmt.Sprintf("<Peer: %s:%d (tasks: %d)>", peer.Address, peer.Port, peer.ActiveTasks)
}

func (peer *Peer) GetAddress() string {
	return peer.Address
}

func (peer *Peer) send(request *Request) Response {
	peer.wlock.Lock()
	request.ReqID = peer.ReqID
	if request.Size != 0 {
		peer.sizes[request.ReqID] = request.Size
	}
	data, _ := msgpack.Marshal(request)
	peer.Connection.Write(data)
	peer.buffers[request.ReqID] = []byte{}
	peer.chans[request.ReqID] = make(chan Response)
	peer.ReqID++
	peer.wlock.Unlock()
	log.WithFields(log.Fields{"request": request}).Info("Sending")
	return <-peer.chans[request.ReqID]
}

func (peer *Peer) Stop() {
	fmt.Println(peer.Address, "stopping")
	peer.Listening = false
	if peer.Ticker != nil {
		peer.Ticker.Stop()
	}
	if peer.Connection != nil {
		peer.Connection.Close()
	}
}

func (peer *Peer) handleAnswers() {
	for {
		if !peer.Listening {
			return
		}
		dl := time.Now().Add(20 * time.Second)
		peer.Connection.SetReadDeadline(dl)
		// fmt.Printf("%s:%d - Set deadline: %s %v\n", peer.Address, peer.Port, dl, peer.Listening)
		message := make([]byte, 1024*16)
		peer.Lock()
		_, _ = peer.Connection.Read(message)
		peer.Unlock()
		answer := Response{}
		msgpack.Unmarshal(message, &answer)
		// log.WithFields(log.Fields{"answer": answer}).Info("Recv")
		// log.WithFields(log.Fields{"rq_id": request.ReqID, "answ_to": answer.To}).Info("Recv")
		if answer.StreamBytes > 0 {
			left := answer.StreamBytes
			buf := peer.buffers[answer.To]
			for left > 0 {
				peer.Lock()
				n, err := peer.Connection.Read(message)
				peer.Unlock()
				if err != nil {
					log.Warn("File streaming error: ", err)
					break
				}
				left = left - n
				buf = append(buf, message...)
			}
			peer.buffers[answer.To] = buf[0:int(math.Min(float64(answer.StreamBytes), float64(len(buf))))]
		}
		peer.Lock()
		answer.Buffer = peer.buffers[answer.To]
		peer.Unlock()
		peer.chans[answer.To] <- answer
	}
}

func (peer *Peer) Download(task interfaces.ITask) ([]byte, error) {
	var res error = nil
	if task.GetStarted() {
		res = errors.New("Parallel")
	}
	task.Start()
	peer.ActiveTasks++
	peer.Tasks = append(peer.Tasks, task)
	filename := path.Join(utils.GetDataPath(), task.GetSite(), task.GetFilename())
	os.MkdirAll(path.Dir(filename), 0777)
	location := 0
	request := Request{
		Cmd: "streamFile",
		Params: RequestFile{
			Site:      task.GetSite(),
			InnerPath: task.GetFilename(),
			Location:  location,
		},
		Size: task.GetSize(),
	}
	message := peer.send(&request)
	content := message.Buffer
	task.AppendContent(message.Buffer, location)
	s := int(peer.sizes[request.ReqID])
	l := len(content)
	if task.GetSize() != 0 && l != int(s) {
		for location+l < s && !task.GetDone() {
			// log.Warn(task, len(message.Buffer), peer.sizes[request.ReqID])
			l = len(content)
			location += l
			request.Params = RequestFile{
				Site:      task.GetSite(),
				InnerPath: task.GetFilename(),
				Location:  location,
			}
			message = peer.send(&request)
			task.AppendContent(message.Buffer, location)
		}
	}
	if len(content) == 0 {
		if !task.Check() {
			log.Fatal(task)
		}
	}
	task.Finish()
	peer.RemoveTask(task)
	peer.ActiveTasks--
	return task.GetContent(), res
}

func (peer *Peer) Ping() {
	ping := Request{
		Cmd:    "ping",
		Params: map[string]string{},
	}
	pong := peer.send(&ping)
	if pong.Body == "Pong!" {
		// fmt.Println("Ping successfull")
	}
}

func (peer *Peer) Handshake() Response {
	hs := Request{
		Cmd: "handshake",
		Params: Handshake{
			Version:        utils.VERSION,
			Rev:            utils.REV,
			Protocol:       "v2",
			PeerID:         utils.GetPeerID(),
			FileserverPort: 0,
			PortOpened:     false,
			TargetIP:       peer.Address,
			CryptSupported: true,
			Crypt:          "tls-rsa",
		},
	}
	return peer.send(&hs)
}

func (peer *Peer) Connect() error {
	peer.State = Connecting
	certFilename := path.Join(utils.GetDataPath(), "cert-rsa.pem")
	keyFilename := path.Join(utils.GetDataPath(), "key-rsa.pem")
	cert, _ := tls.LoadX509KeyPair(certFilename, keyFilename)
	conn, err := tls.DialWithDialer(&net.Dialer{
		Deadline: time.Now().Add(10 * time.Second),
		Timeout:  time.Second * 5,
		Cancel:   peer.Cancel,
	}, "tcp", fmt.Sprintf("%s:%d",
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
		go func() {
			peer.handleAnswers()
		}()
		peer.Handshake()
		peer.Ticker = time.NewTicker(time.Second * 5)
		// go func() {
		// 	for _ = range peer.Ticker.C {
		// 		if peer.Listening {
		// 			peer.Ping()
		// 		} else {
		// 			return
		// 		}
		// 	}
		// }()
	}
	return err
}

func NewPeer(info io.Reader, ch chan *Peer) *Peer {
	addr := [4]byte{}
	port := [2]byte{}
	binary.Read(info, binary.BigEndian, &addr)
	binary.Read(info, binary.BigEndian, &port)
	if port[0] == 0 && port[1] == 0 {
		return nil
	}
	peer := Peer{
		State:       Disconnected,
		Cancel:      make(chan struct{}),
		Listening:   true,
		Address:     fmt.Sprintf("%d.%d.%d.%d", addr[0], addr[1], addr[2], addr[3]),
		Port:        binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, port[0], port[1]}),
		buffers:     map[int][]byte{},
		sizes:       map[int]int64{},
		chans:       map[int]chan Response{},
		ActiveTasks: 0,
		Free:        ch,
		wlock:       &sync.Mutex{},
		ReqID:       r.Intn(1000),
	}
	return &peer
}
