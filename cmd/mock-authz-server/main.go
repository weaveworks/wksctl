// A Kubernetes Webhook Authenticator for Kerboros

package main

import (
	"encoding/json"
	"log"
	"net/http"

	flag "github.com/spf13/pflag"
	authzv1 "k8s.io/api/authorization/v1"
)

func authHandler(w http.ResponseWriter, r *http.Request) {
	// Decode the incoming request
	var sar authzv1.SubjectAccessReview
	err := json.NewDecoder(r.Body).Decode(&sar)
	if err != nil {
		log.Println("[Error]", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log.Println(sar)

	if sar.Spec.User == "admin/admin" {
		sar.Status.Allowed = true
	}

	err = json.NewEncoder(w).Encode(sar)
	if err != nil {
		log.Println("[Error]", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func main() {
	listenAddress := flag.String("listen-address", "127.0.0.1:5001", "address to listen for webhook requests")
	flag.Parse()

	http.HandleFunc("/authorize", authHandler)
	log.Printf("Listening for requests on %s\n", *listenAddress)
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}
