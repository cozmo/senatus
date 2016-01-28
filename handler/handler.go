package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cozmo/senatus/db"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
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
		h.UnknownErrorHandler(res, req, errors.New("No code returned from Google"))
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
		h.UnknownErrorHandler(res, req, err)
		return
	}
	defer postResp.Body.Close()
	tokenBody, err := ioutil.ReadAll(postResp.Body)
	if err != nil {
		h.UnknownErrorHandler(res, req, err)
		return
	}
	var tokenResponse struct {
		AccessToken string `json:"access_token"`
	}
	json.Unmarshal(tokenBody, &tokenResponse)
	accessToken := tokenResponse.AccessToken
	if accessToken == "" {
		h.UnknownErrorHandler(res, req, err)
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
		h.UnknownErrorHandler(res, req, err)
		return
	}
	json.Unmarshal(userBody, &userResponse)
	session, err := h.sessionStore.Get(req, "session")
	if userResponse.Id == "" || userResponse.Name == "" || err != nil {
		h.UnknownErrorHandler(res, req, err)
		return
	}
	session.Values["id"] = userResponse.Id
	session.Values["name"] = userResponse.Name
	err = session.Save(req, res)
	if userResponse.Id == "" || userResponse.Name == "" || err != nil {
		h.UnknownErrorHandler(res, req, err)
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

func (h *Handler) UnknownErrorHandler(res http.ResponseWriter, req *http.Request, err error) {
	fmt.Println(err)
	res.WriteHeader(http.StatusInternalServerError)
	templates.ExecuteTemplate(res, "error.html", map[string]string{"Error": "Unknown Error Occurred"})
}

func (h *Handler) IndexHandler(res http.ResponseWriter, req *http.Request, user *db.User) {
	templates.ExecuteTemplate(res, "index.html", map[string]interface{}{"User": user})
}

func (h *Handler) NewTopicGetHandler(res http.ResponseWriter, req *http.Request, user *db.User) {
	if user == nil {
		h.initiateLogin(req.URL.String(), res, req)
		return
	}
	templates.ExecuteTemplate(res, "newTopic.html", map[string]interface{}{"User": user})
}

func (h *Handler) NewTopicPostHandler(res http.ResponseWriter, req *http.Request, user *db.User) {
	if user == nil {
		h.initiateLogin(req.URL.String(), res, req)
		return
	}
	topic, err := h.database.NewTopic(req.PostFormValue("name"), req.PostFormValue("description"), user)
	if err != nil {
		h.UnknownErrorHandler(res, req, err)
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
		h.UnknownErrorHandler(res, req, err)
		return
	}
	if topic == nil {
		h.NotFoundHandler(res, req)
		return
	}

	_, err = h.database.NewQuestion(mux.Vars(req)["id"], req.PostFormValue("question"), user)
	if err != nil {
		h.UnknownErrorHandler(res, req, err)
		return
	}
	http.Redirect(res, req, "/topics/"+topic.Id, http.StatusFound)
}

func (h *Handler) ViewTopicHandler(res http.ResponseWriter, req *http.Request, user *db.User) {
	topic, err := h.database.TopicById(mux.Vars(req)["id"])
	if err != nil {
		h.UnknownErrorHandler(res, req, err)
		return
	}
	if topic == nil {
		h.NotFoundHandler(res, req)
		return
	}

	questions, err := h.database.QuestionsForTopic(mux.Vars(req)["id"], user)
	if err != nil {
		h.UnknownErrorHandler(res, req, err)
		return
	}

	type questionData struct {
		Id            string
		Text          string
		AuthorName    string
		PostDate      string
		Votes         int
		UserCanVote   bool
		BelongsToUser bool // does the current user own this question?
	}

	processedQuestions := []questionData{}

	for _, question := range questions {
		belongsToUser := false
		if user != nil && user.GoogleId == question.User.GoogleId {
			belongsToUser = true
		}
		q := questionData{
			Id:            question.Id,
			Text:          question.Question,
			AuthorName:    question.User.Username,
			PostDate:      question.Created.Format("Jan 2 2006"),
			Votes:         question.Votes,
			UserCanVote:   question.UserCanVote,
			BelongsToUser: belongsToUser,
		}
		processedQuestions = append(processedQuestions, q)
	}

	isLoggedIn := false
	if user != nil {
		isLoggedIn = true
	}

	data := struct {
		User        *db.User
		LoggedIn    bool
		Id          string
		Name        string
		PostDate    string
		AuthorName  string
		Description string
		Questions   []questionData
	}{
		user, isLoggedIn, topic.Id, topic.Name, topic.Created.Format("Jan 2 2006"), topic.User.Username, topic.Description, processedQuestions,
	}

	templates.ExecuteTemplate(res, "viewTopic.html", data)
}

func (h *Handler) ViewTopicsHandler(res http.ResponseWriter, req *http.Request, user *db.User) {
	if user == nil {
		http.Redirect(res, req, "/", 302)
		return
	}

	topics, err := h.database.TopicsByUser(user)
	if err != nil {
		h.UnknownErrorHandler(res, req, err)
		return
	}

	type topicData struct {
		Id          string
		Name        string
		PostDate    string
		AuthorName  string
		Description string
	}

	processedTopics := []topicData{}

	for _, topic := range topics {
		processedTopics = append(processedTopics, topicData{topic.Id, topic.Name, topic.Created.Format("Jan 2 2006"), topic.User.Username, topic.Description})
	}

	templates.ExecuteTemplate(res, "viewTopics.html", map[string]interface{}{"Topics": processedTopics, "User": user})
}

func (h *Handler) VoteForQuestionHandler(res http.ResponseWriter, req *http.Request, user *db.User) {
	if user == nil {
		http.Redirect(res, req, "/", 302)
		return
	}

	if err := h.database.VoteForQuestion(mux.Vars(req)["question_id"], user); err != nil {
		h.UnknownErrorHandler(res, req, err)
		return
	}
	http.Redirect(res, req, "/topics/"+mux.Vars(req)["id"], 302)
}

func (h *Handler) UnvoteForQuestionHandler(res http.ResponseWriter, req *http.Request, user *db.User) {
	if user == nil {
		http.Redirect(res, req, "/", 302)
		return
	}

	if err := h.database.UnvoteForQuestion(mux.Vars(req)["question_id"], user); err != nil {
		h.UnknownErrorHandler(res, req, err)
		return
	}
	http.Redirect(res, req, "/topics/"+mux.Vars(req)["id"], 302)
}

func (h *Handler) LoginHandler(res http.ResponseWriter, req *http.Request) {
	if mux.Vars(req)["id"] != "" {
		h.initiateLogin("/topics/"+mux.Vars(req)["id"], res, req)
	} else {
		h.initiateLogin("/topics", res, req)
	}
}

func (h *Handler) LogoutHandler(res http.ResponseWriter, req *http.Request) {
	http.SetCookie(res, &http.Cookie{Name: "session", MaxAge: -1, Path: "/"})
	http.Redirect(res, req, "/", 302)
}
