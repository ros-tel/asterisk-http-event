package main

import (
	"bufio"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"

	"github.com/zaf/agi"
	"gopkg.in/yaml.v2"
)

type (
	TConfig struct {
		FastAgiListen struct {
			Host string `yaml:"host"`
			Port string `yaml:"port"`
		} `yaml:"fast_agi_listen"`

		AmoCRM amoCRM `yaml:"amocrm"`

		Database Database `yaml:"database"`
	}

	TVars struct {
		AgentNumber  string
		CallerNumber string
		CalledNumber string
	}

	apiClient struct {
		c      *http.Client
		cookie []*http.Cookie
	}
)

var (
	config *TConfig

	events = make(chan TVars, 100)

	config_file = flag.String("config", "", "Usage: directory <config_file>")
	debug       = flag.Bool("debug", false, "Print debug information on stderr")
)

func main() {
	log.SetFlags(log.Lshortfile)

	flag.Parse()

	// Load the configuration file
	if *config_file == "" {
		*config_file = "config" + string(os.PathSeparator) + "config.yml"
	}

	getConfig(*config_file)

	if *debug {
		log.Printf("[DEBUG] config: %+v", config)
	}

	dbConnect(config.Database)

	AmoServe(config.AmoCRM)

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

	var err error
	myAgi := agi.New()
	rw := bufio.NewReadWriter(bufio.NewReader(client), bufio.NewWriter(client))
	err = myAgi.Init(rw)
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

	var callVars TVars

	rep, err = myAgi.GetVariable("AgentNumber")
	if err != nil {
		if *debug {
			log.Printf("Get AgentNumber error: %+v %+v\n", err, rep)
		}
	} else {
		callVars.AgentNumber = url.QueryEscape(rep.Dat)
	}

	rep, err = myAgi.GetVariable("CallerNumber")
	if err != nil {
		if *debug {
			log.Printf("Get CallerNumber error: %+v\n", err)
		}
	} else {
		callVars.CallerNumber = url.QueryEscape(rep.Dat)
	}

	rep, err = myAgi.GetVariable("CalledNumber")
	if err != nil {
		if *debug {
			log.Printf("Get CalledNumber error: %+v\n", err)
		}
	} else {
		callVars.CalledNumber = url.QueryEscape(rep.Dat)
	}

	if *debug {
		log.Printf("Call Vars: %+v\n", callVars)
	}

	events <- callVars
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
