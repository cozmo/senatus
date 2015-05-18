package db

import (
	"time"
)

type Topic struct {
	Id          string
	Name        string
	Description string
	User        User
	Created     time.Time
}

type User struct {
	GoogleId string
	Username string
}

type Question struct {
	Id       string
	TopicId  string
	Question string
	User     User
	Created  time.Time
}

type DB interface {
	NewTopic(name, description string, user *User) (*Topic, error)
	TopicById(Id string) (*Topic, error)
	QuestionsForTopic(topicId string) ([]*Question, error)
	NewQuestion(topicId, question string) (*Question, error)
}
