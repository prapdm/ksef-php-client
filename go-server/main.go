package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/alapierre/go-ksef-client/ksef/api"
	"github.com/alapierre/go-ksef-client/ksef/model"
	"github.com/joho/godotenv"
)

type KsefClient struct {
	Env            string
	Token          string
	Nip            string
	Client         api.Client
	SessionService api.SessionService
	InvoiceService api.InvoiceService
	SessionToken   string
}

type TokenCache struct {
	Token    string
	IssuedAt int64
}

func (c *KsefClient) reloadSessionToken() {
	sessionToken, isCached := tokens[c.Token]
	if !isCached || sessionToken.IssuedAt+150 < time.Now().Unix() {
		sessionToken, err := c.SessionService.LoginByToken(c.Nip, model.ONIP, c.Token, "keys/"+c.Env+"/publicKey.pem")
		if err != nil {
			fmt.Println("LoginByToken error", err.Error())
		}
		if sessionToken != nil {
			c.SessionToken = sessionToken.SessionToken.Token
			tokens[c.Token] = TokenCache{
				Token:    sessionToken.SessionToken.Token,
				IssuedAt: sessionToken.Timestamp.Unix(),
			}
		}
	}
	sessionToken2, hasToken := tokens[c.Token]
	if hasToken {
		c.SessionToken = sessionToken2.Token
	}
}

func getClient(token, nip, env string) KsefClient {
	cacheKey := token + "-" + nip + "-" + env

	c, ok := clients[cacheKey]

	if !ok {
		var client api.Client
		if env == "prod" {
			client = api.New(api.Prod)
		} else if env == "demo" {
			client = api.New(api.Demo)
		} else {
			client = api.New(api.Test)
		}
		sessionService := api.NewSessionService(client)
		invoiceService := api.NewInvoiceService(client)
		c = KsefClient{
			Env:            env,
			Token:          token,
			Nip:            nip,
			InvoiceService: invoiceService,
			SessionService: sessionService,
			Client:         client,
		}
		clients[cacheKey] = c
	}

	return c
}

func errorMessage(w http.ResponseWriter, message string) {

	data := map[string]interface{}{
		"success": false,
		"message": message,
	}
	out, err := json.Marshal(data)

	if err != nil {
		fmt.Printf("error: %s\n", err.Error())
		io.WriteString(w, "{success: false, message: \"Internal error\"}")
		return
	}

	io.WriteString(w, string(out))
}

func createInvoice(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("x-token")
	nip := r.Header.Get("x-nip")
	env := r.Header.Get("x-env")

	if token == "" || nip == "" || env == "" {
		errorMessage(w, "missing headers")
		return
	} else {
		if env != "test" && env != "prod" && env != "demo" {
			errorMessage(w, "invalid env")
			return
		}
		content, err := io.ReadAll(r.Body)
		if err != nil {
			fmt.Println("Can't read body")
			errorMessage(w, err.Error())
		}
		client := getClient(token, nip, env)
		client.reloadSessionToken()
		invoice, err := client.InvoiceService.SendInvoice(content, client.SessionToken)
		if err != nil {
			fmt.Println("Can't send invoice")
			errorMessage(w, err.Error())
		} else {
			json.NewEncoder(w).Encode(invoice)
		}
	}
}

var clients = map[string]KsefClient{}
var tokens = map[string]TokenCache{}

func main() {

	godotenv.Load()

	socket := os.Getenv("GO_HTTP_SERVER_SOCKET")

	if socket == "" {
		socket = "127.0.0.1:3333"
	}

	fmt.Println("Server starting at", "http://"+socket)

	http.HandleFunc("/create-invoice", createInvoice)

	err2 := http.ListenAndServe(socket, nil)

	if err2 != nil {
		panic(err2)
	}
}
