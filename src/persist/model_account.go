package persist

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"game/deps/fastid"
	"game/deps/kit"
	"game/deps/xlog"
	"game/deps/xtime"
	"game/src/proto/pb"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Status int32

const (
	STATUS_NONE Status = iota
	STATUS_ACTIVE
	STATUS_FALSE
)

type Account struct {
	Account   string          `bson:"account"`             // 账号
	Password  string          `bson:"password"`            // 密码
	LoginType int32           `bson:"loginType,omitempty"` // 登录类型
	AppId     string          `bson:"appId,omitempty"`     // 小程序 appId
	OpenId    string          `bson:"openId,omitempty"`    // 小程序 openId
	UnionId   string          `bson:"unionId,omitempty"`   // 小程序 unionId
	Status    Status          `bson:"Status,omitempty"`    // 状态
	Roles     map[int32]int64 `bson:"Roles"`               // 角色信息
	CreateAt  int64           `bson:"createAt"`            // 创建时间
}

func NewAccount(account string, now int64) *Account {
	acc := &Account{
		Account:  account,
		Status:   STATUS_NONE,
		CreateAt: now,
	}
	return acc
}

// NewMiniProgramAccount 创建一个小程序账号对象，account 字段为 appId/openId 的 sha1 结果
func NewMiniProgramAccount(appId string, openId string, unionId string, now int64) *Account {
	acc := NewAccount(MiniProgramAccountName(appId, openId), now)
	acc.LoginType = int32(pb.AuthLoginType_AUTH_LOGIN_TYPE_CODE2_SESSION)
	acc.AppId = appId
	acc.OpenId = openId
	acc.UnionId = unionId
	return acc
}

// MiniProgramAccountName 生成小程序账号的 account 字段，格式为 "mp_" + sha1(appId + "|" + openId)
func MiniProgramAccountName(appId string, openId string) string {
	sum := sha1.Sum([]byte(appId + "|" + openId))
	return "mp_" + hex.EncodeToString(sum[:])
}

// InsertAccountRole 在指定账号的 Roles 字段中添加一个 roleId-gid 映射
func InsertAccountRole(ctx context.Context, account string, roleId int32, gid int64) bool {
	coll := GetDatabase().Collection(COLLECTION_AUTH_ACCOUNT)
	filter := bson.M{"account": account}
	update := bson.M{"$set": bson.M{"Roles." + kit.Itoa(roleId): gid}}
	_, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		xlog.Warnf("InsertAccountRole error: %v", err)
		return false
	}
	return true
}

// CreateAccount 插入一条新账号记录
func CreateAccount(ctx context.Context, acc *Account) error {
	coll := GetDatabase().Collection(COLLECTION_AUTH_ACCOUNT)
	// 填充创建时间
	if acc.CreateAt == 0 {
		acc.CreateAt = time.Now().Unix()
	}
	acc.Roles = make(map[int32]int64)
	_, err := coll.InsertOne(ctx, acc)
	if mongo.IsDuplicateKeyError(err) {
		return fmt.Errorf("account already Created:%v", acc)
	}
	return err
}

// GetAccount 根据 account 字段查询一条记录
func GetAccount(ctx context.Context, account string) (*Account, error) {
	coll := GetDatabase().Collection(COLLECTION_AUTH_ACCOUNT)
	filter := bson.M{"account": account}
	var result Account
	err := coll.FindOne(ctx, filter).Decode(&result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &result, err
}

// GetMiniProgramAccount 根据 appId/openId 查询小程序账号.
func GetMiniProgramAccount(ctx context.Context, appId string, openId string) (*Account, error) {
	coll := GetDatabase().Collection(COLLECTION_AUTH_ACCOUNT)
	filter := bson.M{"appId": appId, "openId": openId}
	var result Account
	err := coll.FindOne(ctx, filter).Decode(&result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &result, nil
}

// GetOrCreateMiniProgramAccount 获取或创建小程序账号.
func GetOrCreateMiniProgramAccount(ctx context.Context, appId string, openId string, unionId string) (*Account, error) {
	acc, err := GetMiniProgramAccount(ctx, appId, openId)
	if err != nil {
		return nil, err
	}
	if acc == nil {
		acc = NewMiniProgramAccount(appId, openId, unionId, xtime.NowUnix())
		if err := CreateAccount(ctx, acc); err != nil {
			return nil, err
		}
	}
	// 更新账号unionId
	if acc.UnionId == "" && unionId != "" {
		coll := GetDatabase().Collection(COLLECTION_AUTH_ACCOUNT)
		_, err := coll.UpdateOne(ctx, bson.M{"account": acc.Account}, bson.M{"$set": bson.M{"unionId": unionId}})
		if err != nil {
			return nil, err
		}
		acc.UnionId = unionId
	}
	return acc, nil
}

// GetAccountWithPassword 根据 account 和 password 查询账户
func GetAccountWithPassword(ctx context.Context, account string, password string) (acc *Account, err error) {
	coll := GetDatabase().Collection(COLLECTION_AUTH_ACCOUNT)
	filter := bson.M{"account": account}
	var result Account
	err = coll.FindOne(ctx, filter).Decode(&result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			acc = NewAccount(account, xtime.NowUnix())
			acc.Password = password
			if err := CreateAccount(ctx, acc); err != nil {
				return nil, err
			}
			return acc, nil
		}
		xlog.Errorf("GetAccountWithPassword error: %v", err)
		return nil, err // 发生错误
	}

	if result.Password != password {
		return nil, fmt.Errorf("password error")
	}
	return &result, nil // 验证方法有效，账户存在
}

// UpdateAccountStatus 修改某个账号的 Status 字段
func UpdateAccountStatus(ctx context.Context, name string, newStatus Status) error {
	coll := GetDatabase().Collection(COLLECTION_AUTH_ACCOUNT)
	filter := bson.M{"account": name}
	update := bson.M{"$set": bson.M{"Status": newStatus}}
	res, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("account not found:%v", name)
	}
	return nil
}

// DeleteAccountByName 根据 account 字段删除一条记录
func DeleteAccountByName(ctx context.Context, name string) error {
	coll := GetDatabase().Collection(COLLECTION_AUTH_ACCOUNT)
	res, err := coll.DeleteOne(ctx, bson.M{"account": name})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return fmt.Errorf("delete account not found:%v", name)
	}
	return nil
}

// ListAccounts 按条件批量查询（例：按状态或分页）
func ListAccounts(ctx context.Context, statusFilter *Status, limit, skip int64) ([]Account, error) {
	coll := GetDatabase().Collection(COLLECTION_AUTH_ACCOUNT)
	filter := bson.M{}
	if statusFilter != nil {
		filter["Status"] = *statusFilter
	}
	opts := options.Find().
		SetLimit(limit).
		SetSkip(skip).
		SetSort(bson.D{{Key: "createAt", Value: -1}})
	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var list []Account
	if err := cursor.All(ctx, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// 创建玩家
func CreateGamer(ctx context.Context, account string, name string) int64 {
	gid := fastid.GenInt64ID()
	gamer := NewGamerPartData(gid, GamerMainModIndex, GamerBaseModIndex)

	//初始化主信息模块
	main := &pb.Gamer{
		Account: account,
		Id:      gid,
	}
	gamer.Main.SetData(main)
	base := &pb.PlayerBase{
		Gid:  gid,
		Icon: 1,
		Lv:   1,
	}
	gamer.Base.SetData(base)
	//gamer.Device.SetData(deviceInfo)

	err := gamer.Save()
	if err != nil {
		xlog.Warnf("CreateGamer error: %v", err)
		return 0
	}
	InsertAccountRole(ctx, account, 0, gid)

	return gid
}

// ClearAccountRole 根据gamerId查找account，并删除该account中的role
func ClearAccountRole(ctx context.Context, account string, gamerId int64) error {
	coll := GetDatabase().Collection(COLLECTION_AUTH_ACCOUNT)

	acc, err := GetAccount(ctx, account)
	if err != nil {
		return err
	}

	if acc == nil || len(acc.Roles) == 0 {
		return nil
	}

	// 找到对应的roleId并删除
	var roleIdToDelete int32 = -1
	for roleId, gid := range acc.Roles {
		if gid == gamerId {
			roleIdToDelete = roleId
			break
		}
	}

	if roleIdToDelete == -1 {
		xlog.Warnf("ClearAccountRole: gamerId %d not found in Roles for account %s", gamerId, account)
		return nil
	}

	// 删除该roleId
	updateFilter := bson.M{"account": acc.Account}
	update := bson.M{"$unset": bson.M{"Roles." + kit.Itoa(roleIdToDelete): ""}}
	_, err = coll.UpdateOne(ctx, updateFilter, update)
	if err != nil {
		xlog.Errorf("ClearAccountRole UpdateOne error: %v", err)
		return err
	}

	return nil
}
