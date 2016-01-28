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
	Id          string
	TopicId     string
	Question    string
	User        User
	Created     time.Time
	Votes       int
	UserCanVote bool
}

type DB interface {
	NewTopic(name, description string, user *User) (*Topic, error)
	TopicById(Id string) (*Topic, error)
	TopicsByUser(user *User) ([]*Topic, error)
	QuestionsForTopic(topicId string, user *User) ([]*Question, error)
	NewQuestion(topicId, question string, user *User) (*Question, error)
	VoteForQuestion(questionId string, user *User) error
	UnvoteForQuestion(questionId string, user *User) error
}
