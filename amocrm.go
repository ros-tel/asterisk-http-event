package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/ros-tel/amocrm"
)

var (
	contacts = make(chan amocrm.Call, 100)
	leads    = make(chan amocrm.Contact, 100)
)

type (
	amoCRM struct {
		ClientID     string `yaml:"client_id"`
		ClientSecret string `yaml:"client_secret"`
		AuthCode     string `yaml:"auth_code"`
		Domain       string `yaml:"domain"`
		RedirectUrl  string `yaml:"redirect_url"`
		RecordUrl    string `yaml:"record_url"`

		RecordPath string `yaml:"record_path"`

		CreateContactInbound  bool `yaml:"create_contact_inbound"`
		CreateContactOutbound bool `yaml:"create_contact_outbound"`
		CreateLead            bool `yaml:"create_lead"`

		NumberNoUser map[string]int `yaml:"number_to_user"`
	}

	amoC struct {
		cl  amocrm.Client
		cnf amoCRM
	}
)

// Запуск процессов
func AmoServe(cnf amoCRM) {
	storage := new(amocrm.JSONFileTokenStorage)
	storage.File = "amo.tokens"
	aCl := amocrm.NewWithStorage(storage, cnf.ClientID, cnf.ClientSecret, cnf.RedirectUrl)

	err := aCl.SetDomain(cnf.Domain)
	if err != nil {
		log.Println("[ERROR] SetDomain:", err)
		os.Exit(1)
	}

	err = aCl.LoadTokenOrAuthorize(cnf.AuthCode)
	if err != nil {
		log.Println("[ERROR] LoadTokenOrAuthorize:", err)
		os.Exit(1)
	}

	cl := amoC{
		cl:  aCl,
		cnf: cnf,
	}

	go processCalls(cl)
}

// Отправляет событие о звонке
func (cl amoC) amoCrmEvent(event TVars) {
	if *debug {
		log.Printf("[DEBUG] Event: %+v\n", event)
	}

	phone := ""
	if event.CallerNumber != "" {
		phone = event.CallerNumber
	} else if event.CalledNumber != "" {
		phone = event.CalledNumber
	}
	e := amocrm.Event{
		Type:        "phone_call",
		PhoneNumber: phone,
	}

	if event.AgentNumber != "" {
		user, ok := cl.cnf.NumberNoUser[event.AgentNumber]
		if ok {
			e.Users = append(e.Users, user)
		} else {
			if *debug {
				log.Printf("[DEBUG] Not NumberNoUser event: %+v\n", event)
			}
			return
		}
	}

	_, err := cl.cl.EventsV2().Add(
		[]amocrm.Event{
			e,
		},
	)
	if err != nil {
		log.Printf("[ERROR] EventsV2().Add: %+v\n", err)
		return
	}
}

// Отправляет логи о звонках
func (cl amoC) amoCrmCalls(calls []amocrm.Call) bool {
	if *debug {
		log.Printf("[DEBUG] Calls: %+v\n", calls)
	}

	c, e, err := cl.cl.Calls().Create(calls)
	if err != nil {
		log.Printf("[ERROR] Calls().Create: %+v\n", err)
		return false
	}

	// Если нужно создавать контакт
	if config.AmoCRM.CreateContactInbound || config.AmoCRM.CreateContactOutbound {
		for _, ve := range e {
			index, err := strconv.Atoi(ve.RequestID)
			if err != nil {
				log.Printf("[ERROR] strconv.Atoi: %+v %s\n", err, ve.RequestID)
				continue
			}

			call := calls[index]

			if config.AmoCRM.CreateContactInbound && call.Direction == "inbound" {
				contacts <- call
			}
			if config.AmoCRM.CreateContactOutbound && call.Direction == "outbound" {
				contacts <- call
			}
		}
	}

	if *debug {
		log.Printf("[DEBUG] Calls Result %+v %+v", c, e)
	}

	return true
}

// Собираем не отправленные логи о звонках каждую минуту
func processCalls(api amoC) {
	tick := time.Tick(1 * time.Minute)
	for {
		select {
		case <-tick:
			go func() {
				calls := getCallsFromDB()
				if len(calls) > 0 {
					if api.amoCrmCalls(calls) {
						setSendCallToDB(calls)
					}
				}
			}()
		case event := <-events:
			go api.amoCrmEvent(event)
		case call := <-contacts:
			api.amoCrmContactAdd(call)
		case contact := <-leads:
			api.amoCrmLeadAdd(contact)
		}
	}
}

// Создание контакта
func (cl amoC) amoCrmContactAdd(call amocrm.Call) {
	if *debug {
		log.Printf("[DEBUG] Сontact: %+v\n", call)
	}

	contact := amocrm.Contact{
		Name:              "Новый",
		CreatedAt:         call.CreatedAt,
		ResponsibleUserId: call.ResponsibleUserID,
		CustomFieldsValues: []amocrm.FieldValues{
			{
				"field_code": "PHONE",
				"values": []amocrm.FieldValues{
					{
						"value":     call.Phone,
						"enum_code": "WORK",
					},
				},
			},
		},
	}

	result, err := cl.cl.Contacts().Create(
		[]amocrm.Contact{
			contact,
		},
	)
	if err != nil {
		log.Printf("[ERROR] Contacts().Create: %+v\n", err)
		return
	}

	// Если нужно создаем сделку
	if config.AmoCRM.CreateLead {
		if len(result) == 1 {
			contact.Id = result[0].Id
			leads <- contact
		}
	}

	// Отправим лог звонка повторно
	cl.amoCrmCalls(
		[]amocrm.Call{
			call,
		},
	)
}

// Создание сделки
func (cl amoC) amoCrmLeadAdd(contact amocrm.Contact) {
	if *debug {
		log.Printf("[DEBUG] Contact: %+v\n", contact)
	}

	l := amocrm.Lead{
		Name:              "Первичная сделка",
		CreatedAt:         contact.CreatedAt,
		ResponsibleUserId: contact.ResponsibleUserId,
		Embedded: &amocrm.LeadEmbedded{
			Contacts: []amocrm.FieldValues{
				{
					"id": contact.Id,
				},
			},
		},
	}

	_, err := cl.cl.Leads().Create(
		[]amocrm.Lead{
			l,
		},
	)
	if err != nil {
		log.Printf("[ERROR] Leads().Create: %+v\n", err)
		return
	}
}
