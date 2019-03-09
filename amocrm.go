package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"
)

var (
	contacts = make(chan amoCrmCallsAdd, 100)
	leads    = make(chan amoCrmCallsAdd, 100)
)

type (
	amoCRM struct {
		Login     string `yaml:"login"`
		ApiKey    string `yaml:"api_key"`
		BaseUrl   string `yaml:"base_url"`
		RecordUrl string `yaml:"record_url"`

		CreateContact bool `yaml:"create_contact"`
		CreateLead    bool `yaml:"create_lead"`

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

func (cl *apiClient) amoPost(url_tail string, body []byte) (int, []byte, error) {
	req, err := http.NewRequest("POST", config.AmoCRM.BaseUrl+url_tail, bytes.NewReader(body))
	if err != nil {
		return -1, nil, err
	}
	req.Header.Add("Content-Type", "application/json")

	for _, cookie := range cl.cookie {
		req.AddCookie(cookie)
	}

	resp, err := cl.c.Do(req)
	if err != nil {
		return -1, nil, err
	}
	defer resp.Body.Close()

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return -1, nil, err
	}

	return resp.StatusCode, content, nil
}

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

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	if *debug {
		log.Printf("Auth request result: %s\nStatus: %s\nBody response: %s\n", body, resp.Status, content)
	}

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		cl.cookie = resp.Cookies()
		return
	}

	log.Printf("Auth request error: %s\nStatus: %s\nBody response: %s\n", body, resp.Status, content)
}

// Запрашивает печеньки при старте и каждые 14 минут
func processAuth(api *apiClient) {
	// Первая авторизацтя после запуска сразу
	api.amoCrmAuth()

	tick := time.Tick(14 * time.Minute)
	for {
		select {
		case <-tick:
			api.amoCrmAuth()
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

	status_code, content, err := cl.amoPost("/api/v2/events", body)
	if err != nil {
		log.Printf("Error request: %+v\n", err)
		return
	}

	if *debug {
		log.Printf("Event send result:\nBody request: %s\nStatus: %d\nBody response: %s\n", body, status_code, content)
	}

	if status_code != http.StatusAccepted {
		log.Printf("Event send error:\nBody request: %s\nStatus: %d\nBody response: %s\n", body, status_code, content)
	}
}

// Распихивание событий в фоне
func reqBackground(api *apiClient) {
	for {
		select {
		case event := <-events:
			go api.amoCrmEvent(event)
		}
	}
}

type (
	amoCrmCalls struct {
		Add []amoCrmCallsAdd `json:"add"`
	}
	amoCrmCallsAdd struct {
		PhoneNumber string `json:"phone_number"`
		Direction   string `json:"direction"`
		Duration    int    `json:"duration"`
		CreatedAt   int64  `json:"created_at"`
		Link        string `json:"link,omitempty"`
		CallStatus  string `json:"call_status,omitempty"`
		Uniq        string `json:"uniq"`
		Responsible string `json:"responsible_user_id,omitempty"`
		ContactID   string
	}

	amoCrmCallsResponse struct {
		// Links struct {
		//	Self   string `json:"self"`
		//	Method string `json:"method"`
		// } `json:"_links"`
		Embedded struct {
			Errors []struct {
				// Msg  string `json:"msg"`
				Item amoCrmCallsAdd `json:"item"`
				Code int            `json:"code"`
			} `json:"errors"`
		} `json:"_embedded"`
	}
)

// Отправляет логи о звонках
func (cl *apiClient) amoCrmCalls(calls []amoCrmCallsAdd, contact_add bool) bool {
	if *debug {
		log.Printf("Calls: %+v\n", calls)
	}

	e := amoCrmCalls{
		Add: calls,
	}

	body, err := json.Marshal(e)
	if err != nil {
		log.Printf("Error marshal: %+v\n", err)
		return false
	}

	status_code, content, err := cl.amoPost("/api/v2/calls", body)
	if err != nil {
		log.Printf("Error request: %+v\n", err)
		return false
	}

	if *debug {
		log.Printf("Calls send result:\nBody request: %s\nStatus: %d\nBody response: %s\n", body, status_code, content)
	}

	if status_code != http.StatusOK {
		log.Printf("Calls send error:\nBody request: %s\nStatus: %d\nBody response: %s\n", body, status_code, content)
		return false
	}

	if contact_add {
		var r amoCrmCallsResponse
		err = json.Unmarshal(content, &r)
		if err != nil {
			log.Printf("Error unmarshal: %+v\n", err)
			return true
		}

		uniq := ""
		for _, er := range r.Embedded.Errors {
			if er.Code == 263 && uniq != er.Item.Uniq {
				// Номер не найден среди контактов
				contacts <- er.Item
				uniq = er.Item.Uniq
			}
		}
	}

	return true
}

// Собираем не отправленные логи о звонках каждую минуту
func processCalls(api *apiClient) {
	tick := time.Tick(1 * time.Minute)
	for {
		select {
		case <-tick:
			calls := getCallsFromDB()
			if len(calls) > 0 {
				if api.amoCrmCalls(calls, config.AmoCRM.CreateContact) {
					setSendCallToDB(calls)
				}
			}
		case contact := <-contacts:
			api.amoCrmContactAdd(contact)
		case lead := <-leads:
			api.amoCrmLeadAdd(lead)
		}
	}
}

type (
	amoCrmContact struct {
		Add []amoCrmContactAdd `json:"add"`
	}
	amoCrmContactAdd struct {
		Name              string                         `json:"name"`
		CreatedAt         int64                          `json:"created_at"`
		ResponsibleUserID string                         `json:"responsible_user_id,omitempty"`
		CustomFields      []amoCrmContactAddCustomFields `json:"custom_fields"`
	}
	amoCrmContactAddCustomFields struct {
		ID     int                                  `json:"id"`
		Values []amoCrmContactAddCustomFieldsValues `json:"values"`
	}
	amoCrmContactAddCustomFieldsValues struct {
		Value string `json:"value"`
		Enum  string `json:"enum"`
	}

	amoCrmContactResponse struct {
		// Links struct {
		//	Self struct {
		//		Href   string `json:"href"`
		//		Method string `json:"method"`
		//	} `json:"self"`
		// } `json:"_links"`
		Embedded struct {
			Items []struct {
				ID int64 `json:"id"`
				// RequestID int `json:"request_id"`
				// Links     struct {
				//	Self struct {
				//		Href   string `json:"href"`
				//		Method string `json:"method"`
				//	} `json:"self"`
				// } `json:"_links"`
			} `json:"items"`
		} `json:"_embedded"`
	}
)

// Создание контакта
func (cl *apiClient) amoCrmContactAdd(contact amoCrmCallsAdd) {
	if *debug {
		log.Printf("Сontact: %+v\n", contact)
	}

	cacf := amoCrmContactAddCustomFields{
		ID: 408903,
	}
	cacf.Values = append(cacf.Values, amoCrmContactAddCustomFieldsValues{
		Value: contact.PhoneNumber,
		Enum:  "WORK",
	})
	ca := amoCrmContactAdd{
		Name:              "Новый",
		CreatedAt:         contact.CreatedAt,
		ResponsibleUserID: contact.Responsible,
	}
	ca.CustomFields = append(ca.CustomFields, cacf)
	e := amoCrmContact{}
	e.Add = append(e.Add, ca)

	body, err := json.Marshal(e)
	if err != nil {
		log.Printf("Error marshal: %+v\n", err)
		return
	}

	status_code, content, err := cl.amoPost("/api/v2/contacts", body)
	if err != nil {
		log.Printf("Error request: %+v\n", err)
		return
	}

	if *debug {
		log.Printf("Contact send result:\nBody request: %s\nStatus: %d\nBody response: %s\n", body, status_code, content)
	}

	if status_code != http.StatusOK {
		log.Printf("Contact send error:\nBody request: %s\nStatus: %d\nBody response: %s\n", body, status_code, content)
		return
	}

	// Если нужно создаем сделку
	if config.AmoCRM.CreateLead {
		var r amoCrmContactResponse
		err = json.Unmarshal(content, &r)
		if err != nil {
			log.Printf("Error unmarshal: %+v\n", err)
			return
		}
		if len(r.Embedded.Items) == 1 {
			contact.ContactID = strconv.FormatInt(r.Embedded.Items[0].ID, 10)
			leads <- contact
		}
	}

	// Отправим лог звонка повторно
	var c []amoCrmCallsAdd
	c = append(c, contact)
	cl.amoCrmCalls(c, false)
}

type (
	amoCrmLead struct {
		Add []amoCrmLeadAdd `json:"add"`
	}
	amoCrmLeadAdd struct {
		Name              string   `json:"name"`
		CreatedAt         int64    `json:"created_at"`
		StatusID          string   `json:"status_id"`
		ResponsibleUserID string   `json:"responsible_user_id,omitempty"`
		ContactsID        []string `json:"contacts_id"`
	}
)

// Создание сделки
func (cl *apiClient) amoCrmLeadAdd(contact amoCrmCallsAdd) {
	if *debug {
		log.Printf("Lead: %+v\n", contact)
	}

	e := amoCrmLead{
		Add: []amoCrmLeadAdd{
			{
				Name:              "Первичный контакт",
				CreatedAt:         contact.CreatedAt,
				StatusID:          "24516217",
				ResponsibleUserID: contact.Responsible,
				ContactsID:        []string{contact.ContactID},
			},
		},
	}

	body, err := json.Marshal(e)
	if err != nil {
		log.Printf("Error marshal: %+v\n", err)
		return
	}

	status_code, content, err := cl.amoPost("/api/v2/leads", body)
	if err != nil {
		log.Printf("Error request: %+v\n", err)
		return
	}

	if *debug {
		log.Printf("Lead send result:\nBody request: %s\nStatus: %d\nBody response: %s\n", body, status_code, content)
	}

	if status_code != http.StatusOK {
		log.Printf("Lead send error:\nBody request: %s\nStatus: %d\nBody response: %s\n", body, status_code, content)
		return
	}
}
