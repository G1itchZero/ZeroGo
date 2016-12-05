package main

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
	"os"
	"path"
	"time"

	"github.com/Jeffail/gabs"
)

func randomString(n int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[r.Intn(len(letters))]
	}
	return string(b)
}

func createCerts() {
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
	return loadJSON(filename)
}

func loadContent(site string) (*gabs.Container, error) {
	filename := path.Join(".", ZN_PATH, ZN_DATA_ALT, site, "content.json")
	return loadJSON(filename)
}

func loadJSON(filename string) (*gabs.Container, error) {
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

func getTrackers() []string {
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
