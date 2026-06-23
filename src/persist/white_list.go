package persist

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var WhiteListCollection *mongo.Collection

type WhiteList struct {
	ID      bson.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Account string        `bson:"account,omitempty" json:"account,omitempty"`
	DevId   string        `bson:"dev_id,omitempty" json:"devId,omitempty"`
	Ip      string        `bson:"ip,omitempty" json:"ip,omitempty"`
}

func InsertWhiteList(account, devId, ip string) error {
	if account == "" && devId == "" && ip == "" {
		return errors.New("white list must include account, devId, or ip")
	}

	ctx, cancel := context.WithTimeout(context.Background(), mongoOpTimeout)
	defer cancel()

	_, err := GetWhiteListCollection().InsertOne(ctx, &WhiteList{
		Account: account,
		DevId:   devId,
		Ip:      ip,
	})
	return err
}

func IsInWhiteList(account, devId, ip string) (bool, error) {
	conds := bson.A{}
	if account != "" {
		conds = append(conds, bson.M{"account": account})
	}
	if devId != "" {
		conds = append(conds, bson.M{"dev_id": devId})
	}
	if ip != "" {
		conds = append(conds, bson.M{"ip": ip})
	}
	if len(conds) == 0 {
		return false, nil
	}

	filter := bson.M{"$or": conds}
	ctx, cancel := context.WithTimeout(context.Background(), mongoOpTimeout)
	defer cancel()

	var wl WhiteList
	err := GetWhiteListCollection().FindOne(ctx, filter).Decode(&wl)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func ListWhiteList(limit, skip int64) ([]WhiteList, error) {
	ctx, cancel := context.WithTimeout(context.Background(), mongoOpTimeout)
	defer cancel()

	opts := options.Find().SetLimit(limit).SetSkip(skip)
	cursor, err := GetWhiteListCollection().Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var list []WhiteList
	if err := cursor.All(ctx, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func DeleteWhiteList(account, devId, ip string) (int64, error) {
	if account == "" && devId == "" && ip == "" {
		return 0, errors.New("white list delete must include account, devId, or ip")
	}

	ctx, cancel := context.WithTimeout(context.Background(), mongoOpTimeout)
	defer cancel()

	filter := bson.M{}
	if account != "" {
		filter["account"] = account
	}
	if devId != "" {
		filter["dev_id"] = devId
	}
	if ip != "" {
		filter["ip"] = ip
	}

	res, err := GetWhiteListCollection().DeleteMany(ctx, filter)
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}
