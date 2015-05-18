package db

import (
	"errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
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

func (mongodb *MongoDB) NewQuestion(topicId, question string) (*Question, error) {
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
			GoogleId: "abc",
			UserName: "Cosmo Wolfe",
		},
		Created: time.Now(),
	}

	err := session.DB("").C("questions").Insert(mongoquestion)
	return mongoquestion.ToQuestion(), err
}

func (mongodb *MongoDB) QuestionsForTopic(topicId string) ([]*Question, error) {
	if !bson.IsObjectIdHex(topicId) {
		return nil, errors.New("Invalid ObjectId")
	}

	session := mongodb.session.Copy()
	defer session.Close()

	iter := session.DB("").C("questions").Find(bson.M{"topic": bson.ObjectIdHex(topicId)}).Iter()
	var question mongoQuestion
	questions := []*Question{}
	for iter.Next(&question) {
		questions = append(questions, question.ToQuestion())
	}
	if err := iter.Close(); err != nil {
		return questions, err
	}
	return questions, nil
}
