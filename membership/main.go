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

type MembershipApplication struct {
	AppVersion     string
	BackendVersion string
	BackendHost    string
	BackendPort    string
	MembershipAmount     int
	MembershipResult     string
}

func main() {

	port := os.Getenv("PORT")
	if len(port) == 0 {
		port = "8080"
	}

	MembershipApp := MembershipApplication{}

	MembershipApp.AppVersion = os.Getenv("APP_VERSION")
	if len(MembershipApp.AppVersion) == 0 {
		MembershipApp.AppVersion = "dev"
	}

	MembershipApp.BackendHost = os.Getenv("BACKEND_HOST")
	if len(MembershipApp.BackendHost) == 0 {
		MembershipApp.BackendHost = "interest"
	}

	MembershipApp.BackendPort = os.Getenv("BACKEND_PORT")
	if len(MembershipApp.BackendPort) == 0 {
		MembershipApp.BackendPort = "8080"
	}

	// Allow anybody to retrieve version
	http.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, MembershipApp.AppVersion)
	})

	// Kubernetes check if app is ok
	http.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "up")
	})

	// Kubernetes check if app can serve requests
	http.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "yes")
	})

	http.HandleFunc("/", MembershipApp.serveFiles)

	fmt.Printf("Frontend version %s is listening now at port %s\n", MembershipApp.AppVersion, port)
	err := http.ListenAndServe(":"+port, nil)
	log.Fatal(err)
}

func (MembershipApp *MembershipApplication) serveFiles(w http.ResponseWriter, r *http.Request) {
	upath := r.URL.Path
	p := "." + upath
	if p == "./" {
		MembershipApp.home(w, r)
		return
	} else if p == "./diagram.svg" {
		MembershipApp.showDiagram(w, r)
		return
	} else {
		p = filepath.Join("./static/", path.Clean(upath))
	}
	http.ServeFile(w, r, p)
}

func (MembershipApp *MembershipApplication) findBackendVersion() {
	version, err := MembershipApp.callBackend("version")
	if err != nil {
		log.Println("Interest error :", err)
		version = "unknown"
	}

	MembershipApp.BackendVersion = version
}

func (MembershipApp *MembershipApplication) home(w http.ResponseWriter, r *http.Request) {

	MembershipApp.findBackendVersion()
	MembershipApp.handleFormSubmission(w, r)

	t, err := template.ParseFiles("./static/index.html")
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Printf("Error parsing template: %v", err)
		return
	}
	err = t.Execute(w, MembershipApp)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Printf("Error executing template: %v", err)
		return
	}
}

func (MembershipApp *MembershipApplication) showDiagram(w http.ResponseWriter, r *http.Request) {

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
	versionsFound.FV = MembershipApp.AppVersion
	versionsFound.BV = MembershipApp.BackendVersion

	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Accept-Ranges", "bytes")

	err = t.Execute(w, versionsFound)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Printf("Error executing template: %v", err)
		return
	}
}

func (MembershipApp *MembershipApplication) handleFormSubmission(w http.ResponseWriter, r *http.Request) {
	MembershipAmount := parseMembershipAmount(r)
	MembershipApp.MembershipAmount = MembershipAmount
	if MembershipAmount == 0 {
		return
	}

	quote := ""
	interestFound, err := MembershipApp.callBackend("api/v1/interest")
	if err != nil {
		log.Println("Interest error :", err)
		quote = "Could not get interest. Sorry!"
	} else {
		log.Println("Found interest rate " + interestFound)
		interestConverted, _ := strconv.Atoi(interestFound)
		quote = offerQuote(MembershipAmount, interestConverted)
	}
	MembershipApp.MembershipResult = quote

}

func parseMembershipAmount(r *http.Request) int {

	err := r.ParseForm() // Parses the request body
	if err != nil {
		return 0
	}

	MembershipPostParameter := r.Form.Get("Membership") // x will be "" if parameter is not set

	MembershipAmount, err := strconv.Atoi(MembershipPostParameter)
	if err != nil {
		return 0
	}
	return MembershipAmount

}

func offerQuote(Membership int, interest int) string {
	if Membership <= 0 {
		return ""
	}

	total := Membership * interest / 100
	return fmt.Sprintf("With rate %d%% you will pay  %d extra interest", interest, total)

}

func (MembershipApp *MembershipApplication) callBackend(path string) (result string, err error) {

	backendUrl := url.URL{
		Scheme: "http",
		Host:   MembershipApp.BackendHost + ":" + MembershipApp.BackendPort,
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
