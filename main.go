package main

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/cozmo/senatus/db"
	"github.com/cozmo/senatus/handler"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	env "github.com/segmentio/go-env"
)

func isHttps(r *http.Request) bool {
	if r.URL.Scheme == "https" {
		return true
	}
	if strings.HasPrefix(r.Proto, "HTTPS") {
		return true
	}
	if r.Header.Get("X-Forwarded-Proto") == "https" {
		return true
	}
	return false
}

func ensureHttpsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if !isHttps(req) {
			hostToSend := req.Host
			if req.Header.Get("X-Forwarded-Host") != "" {
				hostToSend = req.Header.Get("X-Forwarded-Host")
			}
			http.Redirect(res, req, "https://"+hostToSend+req.URL.String(), 301)
		} else {
			next.ServeHTTP(res, req)
		}
	})
}

func sessionMiddleware(h *handler.Handler, store sessions.Store, routeHandler func(http.ResponseWriter, *http.Request, *db.User)) func(http.ResponseWriter, *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		session, err := store.Get(req, "session")
		if err != nil {
			http.SetCookie(res, &http.Cookie{Name: "session", MaxAge: -1, Path: "/"})
			routeHandler(res, req, nil)
			return
		}
		id, ok := session.Values["id"].(string)
		name, ok := session.Values["name"].(string)
		if id != "" && name != "" && ok {
			user := db.User{id, name}
			routeHandler(res, req, &user)
		} else {
			routeHandler(res, req, nil)
		}
	}
}

func main() {

	requiredEnv := []string{"PORT", "MONGO_URL", "CLIENT_ID", "CLIENT_SECRET", "REDIRECT_URI", "SESSION_SECRET"}
	for _, envVar := range requiredEnv {
		env.MustGet(envVar)
	}

	runtime.GOMAXPROCS(runtime.NumCPU())

	database, err := db.NewMongoDB(os.Getenv("MONGO_URL"))
	if err != nil {
		panic("Error connecting to mongo")
	}

	sessionStore := sessions.NewCookieStore([]byte(os.Getenv("SESSION_SECRET")))

	h := handler.NewHandler(database, sessionStore)

	r := mux.NewRouter()
	r.HandleFunc("/", sessionMiddleware(h, sessionStore, h.IndexHandler)).Methods("GET")
	r.HandleFunc("/topics", sessionMiddleware(h, sessionStore, h.ViewTopicsHandler)).Methods("GET")
	r.HandleFunc("/topics/new", sessionMiddleware(h, sessionStore, h.NewTopicGetHandler)).Methods("GET")
	r.HandleFunc("/topics/new", sessionMiddleware(h, sessionStore, h.NewTopicPostHandler)).Methods("POST")
	r.HandleFunc("/topics/{id}", sessionMiddleware(h, sessionStore, h.ViewTopicHandler)).Methods("GET")
	r.HandleFunc("/topics/{id}/question/{question_id}/vote", sessionMiddleware(h, sessionStore, h.VoteForQuestionHandler)).Methods("POST")
	r.HandleFunc("/topics/{id}/question/{question_id}/unvote", sessionMiddleware(h, sessionStore, h.UnvoteForQuestionHandler)).Methods("POST")
	r.HandleFunc("/login/{id}", h.LoginHandler).Methods("GET")
	r.HandleFunc("/login", h.LoginHandler).Methods("GET")
	r.HandleFunc("/topics/{id}/question", sessionMiddleware(h, sessionStore, h.NewQuestionHandler)).Methods("POST")
	r.HandleFunc("/oauth", h.OAuthCallback).Methods("GET")
	r.HandleFunc("/logout", h.LogoutHandler).Methods("GET")
	r.HandleFunc("/favicon.ico", func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "image/x-icon")
		http.ServeFile(res, req, "./public/favicon.ico")
	}).Methods("GET")
	r.PathPrefix("/public/").Handler(http.StripPrefix("/public/", http.FileServer(http.Dir("./public"))))

	if os.Getenv("ENFORCE_HTTPS") != "" {
		http.Handle("/", ensureHttpsMiddleware(r))
	} else {
		http.Handle("/", r)
	}

	fmt.Println("Checking env...")
	if os.Getenv("APP_ENV") == "development" {
		fmt.Println("Environment is development")
		r.HandleFunc("/dev/login", h.DevLogin).Methods("GET")
	}

	r.HandleFunc("/{url:.*}", h.NotFoundHandler)

	fmt.Println("Serving Senatus on port " + os.Getenv("PORT"))
	http.ListenAndServe(":"+os.Getenv("PORT"), nil)
}
