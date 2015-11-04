package db

import (
	"errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"sort"
	"strings"
	"time"
)

type MongoDB struct {
	session *mgo.Session
}

type mongoUser struct {
	GoogleId string `bson:"google_id"`
	UserName string `bson:"username"`
}

type mongoTopic struct {
	Id          bson.ObjectId `bson:"_id"`
	Name        string        `bson:"name"`
	Description string        `bson:"description"`
	User        mongoUser     `bson:"user"`
	Created     time.Time     `bson:"created"`
}

func (mongotopic mongoTopic) ToTopic() *Topic {
	return &Topic{
		Id:          mongotopic.Id.Hex(),
		Name:        mongotopic.Name,
		Description: mongotopic.Description,
		User: User{
			GoogleId: mongotopic.User.GoogleId,
			Username: mongotopic.User.UserName,
		},
		Created: mongotopic.Created,
	}
}

type mongoQuestion struct {
	Id       bson.ObjectId `bson:"_id"`
	TopicId  bson.ObjectId `bson:"topic"`
	Question string        `bson:"question"`
	User     mongoUser     `bson:"user"`
	Created  time.Time     `bson:"created"`
}

func (mongoquestion mongoQuestion) ToQuestion() *Question {
	return &Question{
		Id:       mongoquestion.Id.Hex(),
		TopicId:  mongoquestion.TopicId.Hex(),
		Question: mongoquestion.Question,
		User: User{
			GoogleId: mongoquestion.User.GoogleId,
			Username: mongoquestion.User.UserName,
		},
		Created: mongoquestion.Created,
	}
}

func NewMongoDB(mongoURL string) (DB, error) {
	session, err := mgo.Dial(mongoURL)
	if err != nil {
		return nil, err
	}
	return &MongoDB{session}, nil
}

func (mongodb *MongoDB) NewTopic(name, description string, user *User) (*Topic, error) {
	if strings.Trim(name, " ") == "" {
		return nil, errors.New("Name must be provided")
	}

	session := mongodb.session.Copy()
	defer session.Close()

	topic := mongoTopic{
		Id:          bson.NewObjectId(),
		Name:        name,
		Description: description,
		User: mongoUser{
			GoogleId: user.GoogleId,
			UserName: user.Username,
		},
		Created: time.Now(),
	}

	err := session.DB("").C("topics").Insert(topic)
	return topic.ToTopic(), err
}

func (mongodb *MongoDB) TopicById(Id string) (*Topic, error) {
	if !bson.IsObjectIdHex(Id) {
		return nil, nil
	}
	session := mongodb.session.Copy()
	defer session.Close()
	result := mongoTopic{}
	query := session.DB("").C("topics").Find(bson.M{"_id": bson.ObjectIdHex(Id)})
	if err := query.One(&result); err != nil {
		return nil, err
	}
	return result.ToTopic(), nil
}

func (mongodb *MongoDB) NewQuestion(topicId, question string, user *User) (*Question, error) {
	if strings.Trim(question, " ") == "" {
		return nil, errors.New("Question must be provided")
	}

	if !bson.IsObjectIdHex(topicId) {
		return nil, errors.New("Invalid ObjectId")
	}

	session := mongodb.session.Copy()
	defer session.Close()

	mongoquestion := mongoQuestion{
		Id:       bson.NewObjectId(),
		TopicId:  bson.ObjectIdHex(topicId),
		Question: question,
		User: mongoUser{
			GoogleId: user.GoogleId,
			UserName: user.Username,
		},
		Created: time.Now(),
	}

	err := session.DB("").C("questions").Insert(mongoquestion)
	return mongoquestion.ToQuestion(), err
}

type sortableQuestions []*Question

func (s sortableQuestions) Len() int {
	return len(s)
}
func (s sortableQuestions) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s sortableQuestions) Less(i, j int) bool {
	if s[i].Votes == s[j].Votes {
		return s[i].Created.Unix() > s[j].Created.Unix()
	} else {
		return s[i].Votes > s[j].Votes
	}
}

func (mongodb *MongoDB) QuestionsForTopic(topicId string, user *User) ([]*Question, error) {
	if !bson.IsObjectIdHex(topicId) {
		return nil, errors.New("Invalid ObjectId")
	}

	session := mongodb.session.Copy()
	defer session.Close()

	iter := session.DB("").C("questions").Find(bson.M{"topic": bson.ObjectIdHex(topicId)}).Iter()
	questions := sortableQuestions{}
	var q mongoQuestion
	for iter.Next(&q) {
		question := q.ToQuestion()
		count, err := session.DB("").C("votes").Find(bson.M{"question": bson.ObjectIdHex(question.Id)}).Count()
		if err != nil {
			continue
		}
		question.Votes = count
		if user != nil {
			count, err = session.DB("").C("votes").Find(bson.M{"question": bson.ObjectIdHex(question.Id), "user.google_id": user.GoogleId}).Count()
			if err != nil {
				continue
			}
			question.UserCanVote = count == 0
		}
		questions = append(questions, question)
	}

	sort.Sort(questions)

	if err := iter.Close(); err != nil {
		return questions, err
	}
	return questions, nil
}

func (mongodb *MongoDB) TopicsByUser(user *User) ([]*Topic, error) {
	session := mongodb.session.Copy()
	defer session.Close()

	iter := session.DB("").C("topics").Find(bson.M{"user.google_id": user.GoogleId}).Iter()
	var topic mongoTopic
	topics := []*Topic{}
	for iter.Next(&topic) {
		topics = append(topics, topic.ToTopic())
	}
	if err := iter.Close(); err != nil {
		return topics, err
	}
	return topics, nil
}

func (mongodb *MongoDB) VoteForQuestion(questionId string, user *User) error {
	session := mongodb.session.Copy()
	defer session.Close()
	if !bson.IsObjectIdHex(questionId) {
		return nil // Fail silently
	}
	vote := bson.M{
		"user": mongoUser{
			GoogleId: user.GoogleId,
			UserName: user.Username,
		},
		"question": bson.ObjectIdHex(questionId),
	}
	op := mgo.Change{
		Update: vote,
		Upsert: true,
	}
	var out mgo.ChangeInfo
	_, err := session.DB("").C("votes").Find(vote).Apply(op, &out)
	return err
}

func (mongodb *MongoDB) UnvoteForQuestion(questionId string, user *User) error {
	session := mongodb.session.Copy()
	defer session.Close()
	if !bson.IsObjectIdHex(questionId) {
		return nil // Fail silently
	}
	query := bson.M{
		"user.google_id": user.GoogleId,
		"question":       bson.ObjectIdHex(questionId),
	}
	var out mgo.ChangeInfo
	_, err := session.DB("").C("votes").Find(query).Apply(mgo.Change{Remove: true}, &out)
	return err
}
