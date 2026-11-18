package api

// Address is an email sender.
type Address struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// CardEmail says a card came by email.
type CardEmail struct {
	From     []string `json:"from"`
	Subject  string   `json:"subject"`
	Language *string  `json:"language"`
}
