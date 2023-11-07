package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"text/template"
)

type OrderApplication struct {
	AppVersion     string
	BackendVersion string
	BackendHost    string
	BackendPort    string
	OrderAmount     int
	OrderResult     string
}

func main() {

	port := os.Getenv("PORT")
	if len(port) == 0 {
		port = "8080"
	}

	orderApp := OrderApplication{}

	orderApp.AppVersion = os.Getenv("APP_VERSION")
	if len(orderApp.AppVersion) == 0 {
		orderApp.AppVersion = "dev"
	}

	orderApp.BackendHost = os.Getenv("BACKEND_HOST")
	if len(orderApp.BackendHost) == 0 {
		orderApp.BackendHost = "interest"
	}

	orderApp.BackendPort = os.Getenv("BACKEND_PORT")
	if len(orderApp.BackendPort) == 0 {
		orderApp.BackendPort = "8080"
	}

	// Allow anybody to retrieve version
	http.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, orderApp.AppVersion)
	})

	// Kubernetes check if app is ok
	http.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "up")
	})

	// Kubernetes check if app can serve requests
	http.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "yes")
	})

	http.HandleFunc("/", orderApp.serveFiles)

	fmt.Printf("Frontend version %s is listening now at port %s\n", orderApp.AppVersion, port)
	err := http.ListenAndServe(":"+port, nil)
	log.Fatal(err)
}

func (orderApp *OrderApplication) serveFiles(w http.ResponseWriter, r *http.Request) {
	upath := r.URL.Path
	p := "." + upath
	if p == "./" {
		orderApp.home(w, r)
		return
	} else if p == "./diagram.svg" {
		orderApp.showDiagram(w, r)
		return
	} else {
		p = filepath.Join("./static/", path.Clean(upath))
	}
	http.ServeFile(w, r, p)
}

func (orderApp *OrderApplication) findBackendVersion() {
	version, err := orderApp.callBackend("version")
	if err != nil {
		log.Println("Interest error :", err)
		version = "unknown"
	}

	orderApp.BackendVersion = version
}

func (orderApp *OrderApplication) home(w http.ResponseWriter, r *http.Request) {

	orderApp.findBackendVersion()
	orderApp.handleFormSubmission(w, r)

	t, err := template.ParseFiles("./static/index.html")
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Printf("Error parsing template: %v", err)
		return
	}
	err = t.Execute(w, orderApp)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Printf("Error executing template: %v", err)
		return
	}
}

func (orderApp *OrderApplication) showDiagram(w http.ResponseWriter, r *http.Request) {

	t, err := template.ParseFiles("./static/diagram.svg")
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Printf("Error parsing template: %v", err)
		return
	}

	type versions struct {
		FV string
		BV string
	}

	versionsFound := versions{}
	versionsFound.FV = orderApp.AppVersion
	versionsFound.BV = orderApp.BackendVersion

	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Accept-Ranges", "bytes")

	err = t.Execute(w, versionsFound)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Printf("Error executing template: %v", err)
		return
	}
}

func (orderApp *OrderApplication) handleFormSubmission(w http.ResponseWriter, r *http.Request) {
	orderAmount := parseOrderAmount(r)
	orderApp.OrderAmount = orderAmount
	if orderAmount == 0 {
		return
	}

	quote := ""
	interestFound, err := orderApp.callBackend("api/v1/interest")
	if err != nil {
		log.Println("Interest error :", err)
		quote = "Could not get interest. Sorry!"
	} else {
		log.Println("Found interest rate " + interestFound)
		interestConverted, _ := strconv.Atoi(interestFound)
		quote = offerQuote(orderAmount, interestConverted)
	}
	orderApp.OrderResult = quote

}

func parseOrderAmount(r *http.Request) int {

	err := r.ParseForm() // Parses the request body
	if err != nil {
		return 0
	}

	orderPostParameter := r.Form.Get("order") // x will be "" if parameter is not set

	orderAmount, err := strconv.Atoi(orderPostParameter)
	if err != nil {
		return 0
	}
	return orderAmount

}

func offerQuote(order int, interest int) string {
	if order <= 0 {
		return ""
	}

	total := order * interest / 100
	return fmt.Sprintf("With rate %d%% you will pay  %d extra interest", interest, total)

}

func (orderApp *OrderApplication) callBackend(path string) (result string, err error) {

	backendUrl := url.URL{
		Scheme: "http",
		Host:   orderApp.BackendHost + ":" + orderApp.BackendPort,
		Path:   path,
	}

	url := backendUrl.String()
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Could not access %s, got %s\n ", url, err)
		return "", errors.New("Could not access " + url)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Println("Non-OK HTTP status:", resp.StatusCode)
		return "", errors.New("Could not access " + url)
	}

	log.Printf("Response status of %s: %s\n", url, resp.Status)

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(resp.Body)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}
