package api

type SignIn struct {
	Name string `json:"name"`
	Pass string `json:"pass"`
}

type SignUp struct {
	Name string `json:"name"`
	Pass string `json:"pass"`
}
