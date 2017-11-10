// Copyright 2017 Yahoo! Holdings Inc. Licensed under the terms of the 3-Clause BSD License.
package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"io"
	"net/http"

	"io/ioutil"
	"os"
	"os/signal"
	"syscall"

	"github.com/Sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	port          = flag.String("port", "443", "Server port.")
	logFilename   = flag.String("logFile", "/var/log/nslifecycle.log", "Log file name and full path.")
	logLevel      = flag.String("logLevel", "info", "The log level.")
	httpsCertFile = flag.String("certFile", "/var/lib/kubernetes/kubernetes.pem", "The cert file for the https server.")
	httpsKeyFile  = flag.String("keyFile", "/var/lib/kubernetes/kubernetes-key.pem", "The key file for the https server.")
	clientCAFile  = flag.String("clientCAFile", "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", "The cluster root CA that signs the apiserver cert")
	clientAuth    = flag.Bool("clientAuth", false, "True to verify client cert/auth during TLS handshake.")
	admitAll      = flag.Bool("admitAll", false, "True to admit all namespace deletions without validation.")

	clientset kubernetes.Interface

	log *logrus.Logger
)

func init() {
	flag.Parse()
	log = getLogger(*logFilename, *logLevel)
}

// statusHandler serves the /status.html response which is always 200.
func statusHandler(rw http.ResponseWriter, req *http.Request) {
	log.Infof("Serving %s %s request for client: %s", req.Method, req.URL.Path, req.RemoteAddr)
	io.WriteString(rw, "OK")
}

func main() {

	// creates the k8s in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Error occurred while building the in-cluster kube-config: %s", err.Error())
	}

	// creates the clientset
	clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error occurred while initializing the client set: %s", err.Error())
	}

	// add the serving path handlers
	mux := http.NewServeMux()
	mux.HandleFunc("/status.html", statusHandler)
	mux.HandleFunc("/", webhookHandler)

	// load the https server cert and key
	xcert, err := tls.LoadX509KeyPair(*httpsCertFile, *httpsKeyFile)
	if err != nil {
		log.Fatalf("Unable to read the server cert and/or key file: %s", err.Error())
	}

	// load the cluster CA that signs the client(apiserver) cert
	caCert, err := ioutil.ReadFile(*clientCAFile)
	if err != nil {
		log.Fatalf("Couldn't load file: %s", err.Error())
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	// create the TLS config for the https server
	tlsConfig := &tls.Config{
		RootCAs:      caCertPool,
		Certificates: []tls.Certificate{xcert},
		ClientCAs:    caCertPool,
	}
	// enable client(apiserver) certificate verification if --clientAuth=true
	if *clientAuth {
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	// create the https server object
	srv := &http.Server{
		Addr:      ":" + *port,
		Handler:   mux,
		TLSConfig: tlsConfig,
	}

	// start the https server
	go func() {
		err = srv.ListenAndServeTLS("", "")
		if err != nil {
			log.Fatal(err)
		}
	}()
	log.Infof("HTTPS server listening on port: %s with ClientAuthEnabled: %t ", *port, *clientAuth)

	// graceful shutdown..
	signalChan := make(chan os.Signal, 2)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	for {
		select {
		case <-signalChan:
			log.Printf("Shutdown signal received, exiting...")
			os.Exit(0)
		}
	}
}
