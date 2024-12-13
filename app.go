package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/oschwald/geoip2-golang"
)

// ConnectionDetails represents comprehensive connection information
type ConnectionDetails struct {
	Request struct {
		RemoteAddr     string            `json:"remote_addr"`
		Host           string            `json:"host"`
		Method         string            `json:"method"`
		UserAgent      string            `json:"user_agent"`
		ForwardedFor   string            `json:"x_forwarded_for"`
		Headers        map[string]string `json:"headers"`
	} `json:"request"`

	Server struct {
		Hostname        string            `json:"hostname"`
		ServerIP        string            `json:"server_ip"`
		Interfaces      map[string]string `json:"network_interfaces"`
	} `json:"server"`

	IPInfo struct {
		PublicIP     string  `json:"public_ip"`
		CountryCode  string  `json:"country_code"`
		Country      string  `json:"country"`
		City         string  `json:"city"`
		Latitude     float64 `json:"latitude"`
		Longitude    float64 `json:"longitude"`
		Organization string  `json:"org"`
		PostalCode   string  `json:"postal_code"`
	} `json:"ip_info"`

	System struct {
		OS struct {
			Platform   string `json:"platform"`
			Arch       string `json:"architecture"`
			GoVersion  string `json:"go_version"`
			CPUNum     int    `json:"cpu_count"`
			Memory     string `json:"total_memory"`
		} `json:"os"`
	} `json:"system"`
}

func getNetworkInterfaces() map[string]string {
	interfaces := make(map[string]string)
	ifaces, err := net.Interfaces()
	if err != nil {
		return interfaces
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			interfaces[iface.Name] = addr.String()
		}
	}
	return interfaces
}

func getPublicIPInfo(ip string) ConnectionDetails {
	details := ConnectionDetails{}
	details.IPInfo.PublicIP = ip

	// Open GeoIP database
	db, err := geoip2.Open("GeoLite2-City.mmdb")
	if err != nil {
		log.Printf("Could not open GeoIP database: %v", err)
		return details
	}
	defer db.Close()

	// Parse IP
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return details
	}

	// Lookup IP
	record, err := db.City(parsedIP)
	if err != nil {
		log.Printf("IP lookup error: %v", err)
		return details
	}

	// Populate IP info
	details.IPInfo.CountryCode = record.Country.IsoCode
	details.IPInfo.Country = record.Country.Names["en"]
	details.IPInfo.City = record.City.Names["en"]
	details.IPInfo.Latitude = record.Location.Latitude
	details.IPInfo.Longitude = record.Location.Longitude
	details.IPInfo.PostalCode = record.Postal.Code

	return details
}

func connectionHandler(w http.ResponseWriter, r *http.Request) {
	// Prepare connection details
	details := ConnectionDetails{}

	// Request details
	details.Request.RemoteAddr = r.RemoteAddr
	details.Request.Host = r.Host
	details.Request.Method = r.Method
	details.Request.UserAgent = r.UserAgent()
	details.Request.ForwardedFor = r.Header.Get("X-Forwarded-For")
	
	// Headers
	details.Request.Headers = make(map[string]string)
	for k, v := range r.Header {
		details.Request.Headers[k] = strings.Join(v, ";")
	}

	// Server details
	hostname, _ := os.Hostname()
	details.Server.Hostname = hostname
	details.Server.Interfaces = getNetworkInterfaces()

	// Get server IP
	addrs, _ := net.InterfaceAddrs()
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				details.Server.ServerIP = ipnet.IP.String()
				break
			}
		}
	}

	// System info
	details.System.OS.Platform = runtime.GOOS
	details.System.OS.Arch = runtime.GOARCH
	details.System.OS.GoVersion = runtime.Version()
	details.System.OS.CPUNum = runtime.NumCPU()

	// Total memory
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	details.System.OS.Memory = humanize.Bytes(m.Sys)

	// IP Info
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = strings.Split(r.RemoteAddr, ":")[0]
	}
	ipDetails := getPublicIPInfo(ip)
	details.IPInfo = ipDetails.IPInfo

	// Determine response type
	acceptHeader := r.Header.Get("Accept")
	isJSON := strings.Contains(acceptHeader, "application/json") || 
			  strings.Contains(r.UserAgent(), "curl")

	if isJSON {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(details)
		return
	}

	// HTML response
	w.Header().Set("Content-Type", "text/html")
	htmlTemplate := `
	<!DOCTYPE html>
	<html>
	<head>
		<title>Connection Details</title>
		<style>
			body { font-family: Arial, sans-serif; max-width: 900px; margin: 0 auto; padding: 20px; }
			pre { background-color: #f4f4f4; padding: 15px; border-radius: 5px; white-space: pre-wrap; word-wrap: break-word; }
		</style>
	</head>
	<body>
		<h1>Connection Details</h1>
		<pre>%s</pre>
	</body>
	</html>`

	jsonOutput, _ := json.MarshalIndent(details, "", "  ")
	fmt.Fprintf(w, htmlTemplate, string(jsonOutput))
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3100"
	}

	http.HandleFunc("/", connectionHandler)
	
	fmt.Printf("Server starting on port %s\n", port)
	log.Fatal(http.ListenAndServe(":" + port, nil))
}
