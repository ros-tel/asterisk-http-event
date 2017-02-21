package main

import (
	"bufio"
	"bytes"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"text/template"
	"time"

	"github.com/zaf/agi"
	"gopkg.in/yaml.v2"
)

type (
	TConfig struct {
		FastAgiListen struct {
			Host string `yaml:"host"`
			Port string `yaml:"port"`
		} `yaml:"fast_agi_listen"`
	}

	TVars struct {
		AgentNumber  string
		CallerNumber string
		CalledNumber string
	}

	apiClient struct {
		c *http.Client
	}
)

var (
	err    error
	config *TConfig
	cl     *apiClient

	config_file = flag.String("config", "", "Usage: directory <config_file>")
	debug       = flag.Bool("debug", false, "Print debug information on stderr")
)

func main() {
	flag.Parse()

	// Load the configuration file
	if *config_file == "" {
		*config_file = "config" + string(os.PathSeparator) + "config.yml"
	}

	getConfig(*config_file)

	cl = &apiClient{
		c: &http.Client{
			Timeout: 20 * time.Second,
			Transport: &http.Transport{
				IdleConnTimeout:     30 * time.Second,
				DisableKeepAlives:   false,
				MaxIdleConnsPerHost: 5,
			},
		},
	}

	fagiserv, err := net.Listen("tcp", config.FastAgiListen.Host+":"+config.FastAgiListen.Port)
	if fagiserv == nil {
		log.Fatal("Cannot listen: %v", err)
	}

	log.Println("Server started")

	for {
		conn, err := fagiserv.Accept()
		if err != nil {
			log.Printf("Accept failed: %v", err)
			continue
		}
		go handleFastAgiConnection(conn)
	}
}

func handleFastAgiConnection(client net.Conn) {
	defer client.Close()

	myAgi := agi.New()
	rw := bufio.NewReadWriter(bufio.NewReader(client), bufio.NewWriter(client))
	err := myAgi.Init(rw)
	if err != nil {
		log.Printf("Error Init: %+v\n", err)
		return
	}

	var rep agi.Reply
	if *debug {
		// Print AGI environment
		log.Println("AGI environment vars:")
		for key, value := range myAgi.Env {
			log.Printf("%-15s: %s\n", key, value)
		}
	}

	network_script, ok := myAgi.Env["network_script"]
	if !ok {
		if *debug {
			log.Println("No variable network_script, exiting")
		}
		return
	}

	tpl, err := template.ParseFiles("template" + string(os.PathSeparator) + network_script + ".tpl")
	if err != nil {
		log.Printf("Template ParseFiles error: %+v\n", err)
		return
	}

	var callVars TVars

	rep, err = myAgi.GetVariable("AgentNumber")
	if err != nil {
		if *debug {
			log.Printf("Get AgentNumber error: %+v %+v\n", err, rep)
		}
	} else {
		callVars.AgentNumber = rep.Dat
	}

	rep, err = myAgi.GetVariable("CallerNumber")
	if err != nil {
		if *debug {
			log.Printf("Get CallerNumber error: %+v\n", err)
		}
	} else {
		callVars.CallerNumber = rep.Dat
	}

	rep, err = myAgi.GetVariable("CalledNumber")
	if err != nil {
		if *debug {
			log.Printf("Get CalledNumber error: %+v\n", err)
		}
	} else {
		callVars.CalledNumber = rep.Dat
	}

	if *debug {
		log.Printf("Call Vars: %+v\n", callVars)
	}

	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	err = tpl.Execute(writer, callVars)
	if err != nil {
		log.Printf("Template Execute error: %+v\n", err)
		return
	}
	writer.Flush()
	if err != nil {
		log.Printf("Writer Flush error: %+v\n", err)
		return
	}
	go func() {
		str, _ := buf.ReadString(0)
		err = cl.request(str)
		if err != nil {
			log.Printf("Error request: %+v\n", err)
		}
	}()
}

func (cl *apiClient) request(url string) error {
	if *debug {
		log.Printf("URL: %+v\n", url)
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := cl.c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	io.Copy(ioutil.Discard, resp.Body)

	return nil
}

// Load the YAML config file
func getConfig(configFile string) {
	var err error
	var input = io.ReadCloser(os.Stdin)
	if input, err = os.Open(configFile); err != nil {
		log.Fatalln(err)
	}
	defer input.Close()

	// Read the config file
	yamlBytes, err := ioutil.ReadAll(input)

	if err != nil {
		log.Fatalln(err)
	}

	// Parse the config
	if err := yaml.Unmarshal(yamlBytes, &config); err != nil {
		//log.Fatalf("Content: %v", yamlBytes)
		log.Fatalf("Could not parse %q: %v", configFile, err)
	}
}
