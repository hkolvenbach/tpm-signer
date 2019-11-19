package tpm

import (
	"crypto"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"sync"

	"github.com/google/go-tpm/tpm2"
	"github.com/google/go-tpm/tpmutil"
)

const ()

var (
	x509Certificate x509.Certificate
	publicKey       crypto.PublicKey
	clientCAs       *x509.CertPool
	clientAuth      *tls.ClientAuthType
)

type TPM struct {
	crypto.Signer
	crypto.Decrypter

	PublicCertFile string
	Certificates   []tls.Certificate
	RootCAs        *x509.CertPool
	ClientCAs      *x509.CertPool
	ClientAuth     tls.ClientAuthType

	TpmHandle    uint32
	TpmDevice    string
	refreshMutex sync.Mutex
}

func NewTPMCrypto(conf *TPM) (TPM, error) {

	rwc, err := tpm2.OpenTPM(conf.TpmDevice)
	if err != nil {
		return TPM{}, fmt.Errorf("google: Public: Unable to Open TPM: %v", err)
	}
	defer func() {
		if err := rwc.Close(); err != nil {
			log.Fatalf("google: Public Unable to close TPM: %v", err)
		}
	}()

	return *conf, nil
}

func (t TPM) TLSCertificate() tls.Certificate {

	rwc, err := tpm2.OpenTPM(t.TpmDevice)
	if err != nil {
		log.Fatalf("google: Public: Unable to Open TPM: %v", err)
	}
	defer func() {
		if err := rwc.Close(); err != nil {
			log.Fatalf("google: Public Unable to close TPM: %v", err)
		}
	}()

	if rwc == nil {
		log.Fatalf("TPM Handle not Initialized.   Run NewTPMCrypt() first")
	}
	if t.PublicCertFile == "" {
		log.Fatalf("Public X509 certificate not specified")
	}

	pubPEM, err := ioutil.ReadFile(t.PublicCertFile)
	if err != nil {
		log.Fatalf("Unable to read keys %v", err)
	}
	block, _ := pem.Decode([]byte(pubPEM))
	if block == nil {
		log.Fatalf("failed to parse PEM block containing the public key")
	}
	pub, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		log.Fatalf("failed to parse public key: " + err.Error())
	}

	x509Certificate = *pub
	var privKey crypto.PrivateKey = t
	return tls.Certificate{
		PrivateKey:  privKey,
		Leaf:        &x509Certificate,
		Certificate: [][]byte{x509Certificate.Raw},
	}
}

func (t TPM) TLSConfig() *tls.Config {

	return &tls.Config{
		Certificates: []tls.Certificate{t.TLSCertificate()},
		RootCAs:      t.RootCAs,
		ClientCAs:    t.ClientCAs,
		ClientAuth:   t.ClientAuth,

		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
		},
		MaxVersion: tls.VersionTLS12,
	}
}

func (t TPM) Public() crypto.PublicKey {
	t.refreshMutex.Lock()
	defer t.refreshMutex.Unlock()

	rwc, err := tpm2.OpenTPM(t.TpmDevice)
	if err != nil {
		log.Fatalf("google: Public: Unable to Open TPM: %v", err)
	}
	defer func() {
		if err := rwc.Close(); err != nil {
			log.Fatalf("google: Public Unable to close TPM: %v", err)
		}
	}()
	if rwc == nil {
		log.Fatalf("TPM Handle not Initialized.   Run NewTPMCrypt() first")
	}
	if publicKey == nil {
		handle := tpmutil.Handle(t.TpmHandle)
		pub, _, _, err := tpm2.ReadPublic(rwc, handle)
		if err != nil {
			log.Fatalf("google: Unable to Read Public data from TPM: %v", err)
		}
		pubKey, err := pub.Key()
		if err != nil {
			log.Fatalf("google: Unable to Read Public key from TPM: %v", err)
		}
		publicKey = pubKey.(*rsa.PublicKey)
	}
	return publicKey
}

func (t TPM) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	t.refreshMutex.Lock()
	defer t.refreshMutex.Unlock()

	rwc, err := tpm2.OpenTPM(t.TpmDevice)
	if err != nil {
		log.Fatalf("google: Public: Unable to Open TPM: %v", err)
	}
	defer func() {
		if err := rwc.Close(); err != nil {
			log.Fatalf("google: Public Unable to close TPM: %v", err)
		}
	}()
	if rwc == nil {
		log.Fatalf("TPM Handle not Initialized.   Run NewTPMCrypt() first")
	}
	hash := opts.HashFunc()
	if len(digest) != hash.Size() {
		return nil, fmt.Errorf("sal: Sign: Digest length doesn't match passed crypto algorithm")
	}
	handle := tpmutil.Handle(t.TpmHandle)
	sig, err := tpm2.Sign(rwc, handle, "", digest, &tpm2.SigScheme{
		Alg:  tpm2.AlgRSASSA,
		Hash: tpm2.AlgSHA256,
	})
	if err != nil {
		return nil, fmt.Errorf("google: Unable to Sign wit TPM: %v", err)
	}
	return []byte(sig.RSA.Signature), nil
}

func (t TPM) Decrypt(rand io.Reader, msg []byte, opts crypto.DecrypterOpts) ([]byte, error) {
	t.refreshMutex.Lock()
	defer t.refreshMutex.Unlock()

	rwc, err := tpm2.OpenTPM(t.TpmDevice)
	if err != nil {
		log.Fatalf("google: Public: Unable to Open TPM: %v", err)
	}
	defer func() {
		if err := rwc.Close(); err != nil {
			log.Fatalf("google: Public Unable to close TPM: %v", err)
		}
	}()

	if rwc == nil {
		log.Fatalf("TPM Handle not Initialized.   Run NewTPMCrypt() first")
	}
	handle := tpmutil.Handle(t.TpmHandle)
	dec, err := tpm2.RSADecrypt(rwc, handle, "", msg, &tpm2.AsymScheme{
		Alg:  tpm2.AlgOAEP,
		Hash: tpm2.AlgSHA256,
	}, "")
	if err != nil {
		return nil, fmt.Errorf("google: Unable to Decrypt with TPM: %v", err)
	}
	return dec, nil
}