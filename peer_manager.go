package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	r "math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/google/go-querystring/query"
	bencode "github.com/jackpal/bencode-go"
)

type Peers []*Peer
type PeerManager struct {
	Address    string
	FreePeers  Peers
	BusyPeers  Peers
	Count      int
	Trackers   []string
	OnPeers    chan *Peer
	OnAnnounce chan int
}

func NewPeerManager(address string) *PeerManager {
	pm := PeerManager{
		Address:    address,
		FreePeers:  Peers{},
		BusyPeers:  Peers{},
		OnPeers:    make(chan *Peer),
		OnAnnounce: make(chan int),
	}
	return &pm
}

func (pm *PeerManager) Get() *Peer {
	if len(pm.FreePeers) == 0 && len(pm.BusyPeers) == 0 {
		return nil
	}
	for len(pm.FreePeers) == 0 {
		time.Sleep(1)
	}
	peer := pm.FreePeers[0]
	return peer
}

func (pm *PeerManager) Free(peer *Peer) {
	pm.removeFreePeer(peer)
	pm.BusyPeers = append(pm.BusyPeers, peer)
}

func (pm *PeerManager) Announce() {
	pm.Trackers = getTrackers()
	for _, tracker := range pm.Trackers {
		go func(tracker string) {
			peers := pm.announceTracker(tracker)
			for _, peer := range peers {
				if pm.peerIsKnown(peer) {
					continue
				}
				go func(peer *Peer) {
					pm.connectPeer(peer)
					if peer.State != Connected {
						return
					}
					pm.FreePeers = append(pm.FreePeers, peer)
					pm.Count++
					pm.OnPeers <- peer
				}(peer)
			}
			fmt.Println(fmt.Sprintf("%s: %d peers", tracker, len(peers)))
			pm.OnAnnounce <- len(peers)
		}(tracker)
	}
}

func (pm *PeerManager) connectPeer(peer *Peer) {
	err := peer.Connect()
	if err == nil {
		// fmt.Println(fmt.Sprintf("Connection established: %s:%d", peer.Address, peer.Port))
		peer.Ping()
	} else {
		// log.WithFields(log.Fields{
		// 	"error": err,
		// 	"peer":  peer,
		// }).Warn("Connection error")
		pm.removeFreePeer(peer)
	}
}

func NewAnnounce(address string, tracker string) Announce {
	return Announce{
		InfoHash: fmt.Sprintf("%s", sha1.Sum([]byte(address))),
		PeerID:   PEER_ID,
		Port:     0, //15441,
		Uploaded: 0, Downloaded: 0,
		Left: 0, Compact: 1, NumWant: 30,
		Event: "started",
	}
}

func (pm *PeerManager) removeFreePeer(peer *Peer) {
	i := func() int {
		i := 0
		for _, b := range pm.FreePeers {
			if b.Address == peer.Address {
				return i
			}
			i++
		}
		return -1
	}()
	if i == -1 {
		return
	}
	pm.FreePeers = append(pm.FreePeers[:i], pm.FreePeers[i+1:]...)
}

func (pm *PeerManager) removeBusyPeer(peer *Peer) {
	i := func() int {
		i := 0
		for _, b := range pm.BusyPeers {
			if b.Address == peer.Address {
				return i
			}
			i++
		}
		return -1
	}()
	if i == -1 {
		return
	}
	pm.BusyPeers = append(pm.BusyPeers[:i], pm.BusyPeers[i+1:]...)
}

func (pm *PeerManager) peerIsKnown(peer *Peer) bool {
	for _, b := range pm.FreePeers {
		if b.Address == peer.Address {
			return true
		}
	}
	return false
}

func (pm *PeerManager) announceHTTP(tracker string) Peers {
	params, _ := query.Values(NewAnnounce(pm.Address, tracker))
	url := fmt.Sprintf("%s?%s", tracker, params.Encode())
	log.WithFields(log.Fields{
		"tracker": tracker,
		"params":  params,
	}).Info("Announce tracker")
	resp, _ := http.Get(url)
	raw, _ := bencode.Decode(resp.Body)
	resp.Body.Close()
	data := raw.(map[string]interface{})
	peerData, _ := GetBytes(data["peers"])
	peerReader := bytes.NewReader(peerData)
	peerCount := len(peerData) / 6
	peers := Peers{}
	for i := 0; i < peerCount; i++ {
		peer := NewPeer(peerReader)
		if peer.Port == 0 {
			continue
		}
		peers = append(peers, peer)
	}
	return peers
}

const (
	UDP_REQUEST_CONNECT  = 0
	UDP_REQUEST_ANNOUNCE = 1
)

func (pm *PeerManager) announceUDP(tracker string) Peers {
	serverAddr, _ := net.ResolveUDPAddr("udp", strings.Replace(tracker, "udp://", "", 1))
	connectionID := int64(0x41727101980)
	rnd := r.New(r.NewSource(time.Now().UnixNano()))
	port := int32(rnd.Intn(99) + 6800)
	transactionID := rnd.Int31()
	sid := "-ZN" + strconv.Itoa(os.Getpid()) + "_" + strconv.FormatInt(rnd.Int63(), 10)
	peerID := []byte(sid[0:20])
	infoHash := sha1.Sum([]byte(pm.Address))
	// fmt.Println(fmt.Sprintf("%x", sha1.Sum([]byte(d.Address))))
	conn, err := net.ListenPacket("udp4", fmt.Sprintf(":%d", port))
	if err != nil {
		return Peers{}
	}
	socket := conn.(*net.UDPConn)
	defer socket.Close()

	socket.SetDeadline(time.Now().Add(20 * time.Second))

	request := new(bytes.Buffer)
	var data = []interface{}{
		connectionID,
		uint32(UDP_REQUEST_CONNECT),
		transactionID,
	}
	for _, v := range data {
		binary.Write(request, binary.BigEndian, v)
	}
	socket.WriteToUDP(request.Bytes(), serverAddr)
	buf := make([]byte, 16)
	buf2 := make([]byte, 10240)
	n, addr, err := socket.ReadFromUDP(buf)
	fmt.Println("Received ", fmt.Sprintf("%x", buf[0:n]), " from ", addr)
	if err != nil {
		fmt.Println("Error: ", err)
		return Peers{}
	}
	answer := bytes.NewReader(buf)
	var resp_a int32
	var resp_t int32
	var resp_c int64
	binary.Read(answer, binary.BigEndian, &resp_a)
	binary.Read(answer, binary.BigEndian, &resp_t)
	binary.Read(answer, binary.BigEndian, &resp_c)
	// fmt.Printf("a< %x %x \n", resp_a, resp_c)
	// fmt.Println(resp_a, resp_t, resp_c)
	announce := new(bytes.Buffer)
	data = []interface{}{
		resp_c,
		uint32(UDP_REQUEST_ANNOUNCE),
		transactionID,
		infoHash,
		peerID,
		int64(0),
		int64(0),
		int64(0),
		int32(2),
		int32(0),
		int32(0),
		int32(50),
		int16(port),
	}

	for _, v := range data {
		binary.Write(announce, binary.BigEndian, v)
	}
	fmt.Println(fmt.Sprintf("%x", announce.Bytes()))
	socket.WriteToUDP(announce.Bytes(), serverAddr)
	n, addr, err = socket.ReadFromUDP(buf2)
	buf2 = buf2[0:n]
	// fmt.Println("Received ", fmt.Sprintf("%x", buf2), " from ", addr, tracker)
	if err != nil {
		fmt.Println("Error: ", err)
		return Peers{}
	}
	answer = bytes.NewReader(buf2)
	var a uint32
	var t uint32
	var i uint32
	var l uint32
	var s uint32
	binary.Read(answer, binary.BigEndian, &a)
	binary.Read(answer, binary.BigEndian, &t)
	binary.Read(answer, binary.BigEndian, &i)
	binary.Read(answer, binary.BigEndian, &l)
	binary.Read(answer, binary.BigEndian, &s)
	peers := Peers{}
	for answer.Len() > 0 {
		peer := NewPeer(answer)
		if peer.Port == 0 {
			continue
		}
		peers = append(peers, peer)
	}
	return peers
}

func (pm *PeerManager) announceTracker(tracker string) Peers {
	pm.Trackers = append(pm.Trackers, tracker)
	if strings.HasPrefix(tracker, "http://") {
		return pm.announceHTTP(tracker)
	} else if strings.HasPrefix(tracker, "udp://") {
		return pm.announceUDP(tracker)
	}
	return Peers{}
}
