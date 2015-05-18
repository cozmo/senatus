package handler

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/templaedhel/senatus/db"
	"html/template"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
)

type Handler struct {
	database     db.DB
	sessionStore sessions.Store
}

func NewHandler(database db.DB, sessionStore sessions.Store) *Handler {
	return &Handler{database, sessionStore}
}

var templates = template.Must(template.ParseGlob("./templates/*"))

func (h *Handler) initiateLogin(destination string, res http.ResponseWriter, req *http.Request) {

	session, err := h.sessionStore.Get(req, "session")
	if err == nil {
		session.Values["destination"] = destination
		session.Save(req, res)
	}

	v := url.Values{}
	v.Set("response_type", "code")
	v.Set("redirect_uri", os.Getenv("REDIRECT_URI"))
	v.Set("client_id", os.Getenv("CLIENT_ID"))
	v.Set("scope", "profile")

	http.Redirect(res, req, "https://accounts.google.com/o/oauth2/auth?"+v.Encode(), 302)
}

func (h *Handler) OAuthCallback(res http.ResponseWriter, req *http.Request) {
	codes := req.URL.Query()["code"]
	if len(codes) != 1 {
		h.UnknownErrorHandler(res, req)
		return
	}
	code := codes[0]
	values := url.Values{}
	values.Set("code", code)
	values.Set("grant_type", "authorization_code")
	values.Set("redirect_uri", os.Getenv("REDIRECT_URI"))
	values.Set("client_id", os.Getenv("CLIENT_ID"))
	values.Set("client_secret", os.Getenv("CLIENT_SECRET"))
	postResp, err := http.PostForm("https://www.googleapis.com/oauth2/v3/token", values)
	if err != nil {
		h.UnknownErrorHandler(res, req)
		return
	}
	defer postResp.Body.Close()
	tokenBody, err := ioutil.ReadAll(postResp.Body)
	if err != nil {
		h.UnknownErrorHandler(res, req)
		return
	}
	var tokenResponse struct {
		AccessToken string `json:"access_token"`
	}
	json.Unmarshal(tokenBody, &tokenResponse)
	accessToken := tokenResponse.AccessToken
	if accessToken == "" {
		h.UnknownErrorHandler(res, req)
		return
	}
	userReq, _ := http.NewRequest("GET", "https://www.googleapis.com/plus/v1/people/me", nil)
	userReq.Header.Set("Authorization", "Bearer "+accessToken)
	client := http.Client{}
	userResp, _ := client.Do(userReq)
	defer userResp.Body.Close()
	var userResponse struct {
		Id   string `json:"id"`
		Name string `json:"displayName"`
	}
	userBody, err := ioutil.ReadAll(userResp.Body)
	if err != nil {
		h.UnknownErrorHandler(res, req)
		return
	}
	json.Unmarshal(userBody, &userResponse)
	session, err := h.sessionStore.Get(req, "session")
	if userResponse.Id == "" || userResponse.Name == "" || err != nil {
		h.UnknownErrorHandler(res, req)
		return
	}
	session.Values["id"] = userResponse.Id
	session.Values["name"] = userResponse.Name
	err = session.Save(req, res)
	if userResponse.Id == "" || userResponse.Name == "" || err != nil {
		h.UnknownErrorHandler(res, req)
		return
	}
	str, ok := session.Values["destination"].(string)
	if ok {
		http.Redirect(res, req, str, 302)
	} else {
		http.Redirect(res, req, "/", 302)
	}
}

func (h *Handler) NotFoundHandler(res http.ResponseWriter, req *http.Request) {
	res.WriteHeader(http.StatusNotFound)
	templates.ExecuteTemplate(res, "error.html", map[string]string{"Error": "Invalid URL"})
}

func (h *Handler) UnknownErrorHandler(res http.ResponseWriter, req *http.Request) {
	res.WriteHeader(http.StatusInternalServerError)
	templates.ExecuteTemplate(res, "error.html", map[string]string{"Error": "Unknown Error Occurred"})
}

func (h *Handler) IndexHandler(res http.ResponseWriter, req *http.Request, user *db.User) {
	templates.ExecuteTemplate(res, "index.html", nil)
}

func (h *Handler) NewTopicGetHandler(res http.ResponseWriter, req *http.Request, user *db.User) {
	if user == nil {
		h.initiateLogin(req.URL.String(), res, req)
		return
	}
	templates.ExecuteTemplate(res, "newTopic.html", nil)
}

func (h *Handler) NewTopicPostHandler(res http.ResponseWriter, req *http.Request, user *db.User) {
	if user == nil {
		h.initiateLogin(req.URL.String(), res, req)
		return
	}
	topic, err := h.database.NewTopic(req.PostFormValue("name"), req.PostFormValue("description"), user)
	if err != nil {
		h.UnknownErrorHandler(res, req)
		return
	}
	http.Redirect(res, req, "/topics/"+topic.Id, http.StatusFound)
}

func (h *Handler) NewQuestionHandler(res http.ResponseWriter, req *http.Request, user *db.User) {
	if user == nil {
		h.initiateLogin("/topics/"+mux.Vars(req)["id"], res, req)
		return
	}
	topic, err := h.database.TopicById(mux.Vars(req)["id"])
	if err != nil {
		h.UnknownErrorHandler(res, req)
		return
	}
	if topic == nil {
		h.NotFoundHandler(res, req)
		return
	}

	_, err = h.database.NewQuestion(mux.Vars(req)["id"], req.PostFormValue("question"))
	if err != nil {
		h.UnknownErrorHandler(res, req)
		return
	}
	http.Redirect(res, req, "/topics/"+topic.Id, http.StatusFound)
}

func (h *Handler) ViewTopicHandler(res http.ResponseWriter, req *http.Request, user *db.User) {
	topic, err := h.database.TopicById(mux.Vars(req)["id"])
	if err != nil {
		h.UnknownErrorHandler(res, req)
		return
	}
	if topic == nil {
		h.NotFoundHandler(res, req)
		return
	}

	questions, err := h.database.QuestionsForTopic(mux.Vars(req)["id"])
	if err != nil {
		h.UnknownErrorHandler(res, req)
		return
	}

	type questionData struct {
		Text       string
		AuthorName string
		PostDate   string
	}

	processedQuestions := []questionData{}

	for _, question := range questions {
		processedQuestions = append(processedQuestions, questionData{question.Question, question.User.Username, question.Created.Format("Jan 2 2006")})
	}

	isLoggedIn := false
	if user != nil {
		isLoggedIn = true
	}

	data := struct {
		LoggedIn    bool
		Id          string
		Name        string
		PostDate    string
		AuthorName  string
		Description string
		Questions   []questionData
	}{
		isLoggedIn, topic.Id, topic.Name, topic.Created.Format("Jan 2 2006"), topic.User.Username, topic.Description, processedQuestions,
	}

	templates.ExecuteTemplate(res, "viewTopic.html", data)
}

func (h *Handler) LoginHandler(res http.ResponseWriter, req *http.Request) {
	h.initiateLogin("/topics/"+mux.Vars(req)["id"], res, req)
}
