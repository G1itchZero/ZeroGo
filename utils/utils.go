package utils

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/gob"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	r "math/rand"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/Jeffail/gabs"
)

var PEER_ID string
var DATA string

// ZeroNet version mimicry
const VERSION string = "0.5.1"
const REV int = 1756

const ZN_PATH string = "ZeroNet"
const ZN_DATA string = "data"
const ZN_DATA_ALT string = "data_alt"
const ZN_UPDATE string = "1UPDatEDxnvHDo7TXvq6AEBARfNkyfxsp"
const ZN_HOMEPAGE string = "1HeLLo4uzjaLetFx6NH3PMwFP3qbRbTf3D"
const ZN_MEDIA string = "ZeroNet/src/Ui/media"
const ZN_NAMES string = "1Name2NXVi1RDPDgf5617UoW7xA6YrhM9F"
const ZN_ID string = "1iD5ZQJMNXu43w1qLB8sfdHVKppVMduGz"

var homepage string

func SetHomepage(hp string) {
	homepage = hp
}

func GetHomepage() string {
	return homepage
}

func GetDataPath() string {
	if DATA == "" {
		DATA = path.Join(".", "data")
	}
	return DATA
}

func GetPeerID() string {
	if PEER_ID == "" {
		PEER_ID = fmt.Sprintf("-ZN0%s-GO%s", strings.Replace(VERSION, ".", "", -1), RandomString(10))
	}
	return PEER_ID
}

func RandomString(n int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[r.Intn(len(letters))]
	}
	return string(b)
}

func CreateCerts() {
	template := &x509.Certificate{
		IsCA: true,
		BasicConstraintsValid: true,
		SubjectKeyId:          []byte{4, 8, 3},
		SerialNumber:          big.NewInt(9899),
		Subject: pkix.Name{
			Country:      []string{"Earth"},
			Organization: []string{"Mother Nature"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().AddDate(5, 5, 5),
		// see http://golang.org/pkg/crypto/x509/#KeyUsage
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
	}

	// generate private key
	privatekey, err := rsa.GenerateKey(rand.Reader, 2048)

	if err != nil {
		fmt.Println(err)
	}

	publickey := &privatekey.PublicKey

	// create a self-signed certificate. template = parent
	var parent = template
	cert, err := x509.CreateCertificate(rand.Reader, template, parent, publickey, privatekey)

	if err != nil {
		fmt.Println(err)
	}

	// save private key
	pemfile, _ := os.Create(path.Join(DATA, "key-rsa.pem"))
	var pemkey = &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privatekey)}
	pem.Encode(pemfile, pemkey)
	pemfile.Close()

	pemfile, _ = os.Create(path.Join(DATA, "cert-rsa.pem"))
	pemkey = &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert}
	pem.Encode(pemfile, pemkey)
	pemfile.Close()
}

func loadUsers() (*gabs.Container, error) {
	filename := path.Join(".", ZN_PATH, ZN_DATA, "users.json")
	return LoadJSON(filename)
}

func loadContent(site string) (*gabs.Container, error) {
	filename := path.Join(".", ZN_PATH, ZN_DATA_ALT, site, "content.json")
	return LoadJSON(filename)
}

func LoadJSON(filename string) (*gabs.Container, error) {
	content, err := ioutil.ReadFile(filename)
	if err == nil {
		return gabs.ParseJSON(content)
	}
	return nil, errors.New("cant read file")
}

func GetBytes(key interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(key)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func GetTrackers() []string {
	trackers := []string{
		// "zero://boot3rdez4rzn36x.onion:15441",
		// "zero://boot.zeronet.io#f36ca555bee6ba216b14d10f38c16f7769ff064e0e37d887603548cc2e64191d:15441",
		"udp://tracker.coppersurfer.tk:6969",
		"udp://tracker.leechers-paradise.org:6969",
		"udp://9.rarbg.com:2710",
		"http://tracker.tordb.ml:6881/announce",
		"http://explodie.org:6969/announce",
		"http://tracker1.wasabii.com.tw:6969/announce",
	}
	return trackers
}

func Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

var IP string

func GetExternalIP() string {
	if IP != "" {
		return IP
	}
	resp, _ := http.Get("http://myexternalip.com/raw")
	defer resp.Body.Close()
	ip, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "0.0.0.0"
	}
	IP = strings.Trim(string(ip), "\n ")
	return IP
}
