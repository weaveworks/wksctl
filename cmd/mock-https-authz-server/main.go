// A Kubernetes Webhook Authenticator for Kerboros

package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"path/filepath"

	flag "github.com/spf13/pflag"
	authnv1 "k8s.io/api/authentication/v1"
	authzv1 "k8s.io/api/authorization/v1"
)

func authorizeHandler(w http.ResponseWriter, r *http.Request) {
	// Decode the incoming request
	var sar authzv1.SubjectAccessReview
	err := json.NewDecoder(r.Body).Decode(&sar)
	if err != nil {
		log.Println("[Error]", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log.Println(sar)

	if sar.Spec.ResourceAttributes == nil || sar.Spec.ResourceAttributes.Namespace != "kittensandponies" {
		sar.Status.Allowed = true
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(sar)
}

func authenticateHandler(w http.ResponseWriter, r *http.Request) {
	var tr authnv1.TokenReview
	err := json.NewDecoder(r.Body).Decode(&tr)
	if err != nil {
		log.Println("[Error]", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log.Println(tr)
	tr.Status.Authenticated = true
}

func main() {
	listenAddress := flag.String("listen-address", "127.0.0.1:5001", "address to listen for webhook requests")
	pemDir := flag.String("pem-dir", ".", "where to find pem files")
	flag.Parse()
	pemFile := func(filename string) string {
		return filepath.Join(*pemDir, filename)
	}
	caCertPEM, _ := ioutil.ReadFile(pemFile("rootCA.pem"))
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(caCertPEM)
	server := &http.Server{
		Addr:      *listenAddress,
		TLSConfig: &tls.Config{},
	}

	http.HandleFunc("/authorize", authorizeHandler)
	http.HandleFunc("/authenticate", authenticateHandler)
	log.Printf("Listening for requests on %s\n", *listenAddress)
	log.Fatal(server.ListenAndServeTLS(pemFile("server.crt"), pemFile("server.key")))
}
