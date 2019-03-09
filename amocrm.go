package main

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

type (
	amoCRM struct {
		Login   string `yaml:"login"`
		ApiKey  string `yaml:"api_key"`
		BaseUrl string `yaml:"base_url"`

		NumberNoUser map[string]string `yaml:"number_to_user"`
	}

	amoCrmAuth struct {
		Login  string `json:"USER_LOGIN"`
		ApiKey string `json:"USER_HASH"`
	}

	amoCrmEvent struct {
		Add []amoCrmEventAdd `json:"add"`
	}
	amoCrmEventAdd struct {
		PhoneNumber string   `json:"phone_number"`
		Type        string   `json:"type"`
		Users       []string `json:"users"`
	}
)

var (
	amoCookies []*http.Cookie
)

// Получение печенек
func (cl *apiClient) amoCrmAuth() {
	body, err := json.Marshal(amoCrmAuth{
		Login:  config.AmoCRM.Login,
		ApiKey: config.AmoCRM.ApiKey,
	})
	if err != nil {
		log.Printf("Error marshal: %+v\n", err)
		return
	}

	req, err := http.NewRequest("POST", config.AmoCRM.BaseUrl+"/private/api/auth.php?type=json", bytes.NewReader(body))
	if err != nil {
		log.Printf("Error request: %+v\n", err)
		return
	}
	req.Header.Add("Content-Type", "application/json")
	resp, err := cl.c.Do(req)
	if err != nil {
		log.Printf("Error request: %+v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		amoCookies = resp.Cookies()
	} else {
		log.Println("Bad auth")
		content, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return
		}
		log.Printf("Body request: %s\nStatus: %s\nBody response: %s\n", body, resp.Status, content)
		return
	}

	io.Copy(ioutil.Discard, resp.Body)
}

// Запрашивает печеньки при старте и каждые 14 минут
func processAuth() {
	cl := &apiClient{
		c: &http.Client{
			Timeout: 20 * time.Second,
			Transport: &http.Transport{
				IdleConnTimeout:     30 * time.Second,
				DisableKeepAlives:   false,
				MaxIdleConnsPerHost: 5,
			},
		},
	}

	cl.amoCrmAuth()

	tick := time.Tick(14 * time.Minute)
	for {
		select {
		case <-tick:
			cl.amoCrmAuth()
		}
	}
}

// Отправляет событие о звонке
func (cl *apiClient) amoCrmEvent(event TVars) {
	if *debug {
		log.Printf("Event: %+v\n", event)
	}

	phone := ""
	if event.CallerNumber != "" {
		phone = event.CallerNumber
	} else if event.CalledNumber != "" {
		phone = event.CalledNumber
	}
	ea := amoCrmEventAdd{
		Type:        "phone_call",
		PhoneNumber: phone,
	}
	if event.AgentNumber != "" {
		if user, ok := config.AmoCRM.NumberNoUser[event.AgentNumber]; ok {
			ea.Users = append(ea.Users, user)
		}
	}
	e := amoCrmEvent{}
	e.Add = append(e.Add, ea)

	body, err := json.Marshal(e)
	if err != nil {
		log.Printf("Error marshal: %+v\n", err)
		return
	}

	req, err := http.NewRequest("POST", config.AmoCRM.BaseUrl+"/api/v2/events", bytes.NewReader(body))
	if err != nil {
		log.Printf("Error request: %+v\n", err)
		return
	}
	req.Header.Add("Content-Type", "application/json")

	for _, c := range amoCookies {
		req.AddCookie(c)
	}

	resp, err := cl.c.Do(req)
	if err != nil {
		log.Printf("Error request: %+v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		log.Println("Bad event")
		content, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return
		}
		log.Printf("Body request: %s\nStatus: %s\nBody response: %s\n", body, resp.Status, content)
		return
	}

	io.Copy(ioutil.Discard, resp.Body)
}

// Распихивание событий в фоне
func reqBackground() {
	cl := &apiClient{
		c: &http.Client{
			Timeout: 20 * time.Second,
			Transport: &http.Transport{
				IdleConnTimeout:     30 * time.Second,
				DisableKeepAlives:   false,
				MaxIdleConnsPerHost: 5,
			},
		},
	}

	for {
		select {
		case event := <-events:
			go cl.amoCrmEvent(event)
		}
	}
}
