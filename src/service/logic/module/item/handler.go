package item

import (
	"game/deps/msg"
	"game/deps/netmgr"
	"game/deps/xlog"
	"game/src/common"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/proto/pbrpc"
	"game/src/service/logic/actor"
	"game/src/service/logic/iface"
	"time"

	"google.golang.org/protobuf/proto"
)

const gamerRpcWaitTimeout = 20 * time.Second

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) HandleRpcGamerAddItem(_ netmgr.IMsgQue, msg *msg.Message) *pbrpc.S2SRpcRSP {
	req := msg.Message().(*pbrpc.S2SAddItemREQ)
	rsp := &pbrpc.S2SRpcRSP{
		RspType: &pbrpc.S2SRpcRSP_AddItemRsp{AddItemRsp: &pbrpc.S2SAddItemRSP{}},
	}
	gamer := actor.FindGamerWithGid(req.Gid)
	if gamer == nil {
		xlog.Warnf("reqAddGamerItem: gamer not found gid:%v", req.Gid)
		rsp.Error = &errorpb.RpcError{ErrCode: errorpb.ERROR_RPC_GAMER_NOT_FOUND, ErrDesc: "gamer not found"}
		return rsp
	}
	type addItemResult struct {
		items []*pb.Item
		err   errorpb.ERROR
	}
	ch := make(chan addItemResult, 1)

	if err := gamer.AddMsgTask(pb.MSG_ID(msg.MsgId()), func() {
		items := make([]*pb.Item, 0, len(req.Items))
		for _, item := range req.Items {
			items = append(items, item)
		}
		reason := common.BuildReason(req.Reason, "", req.ReasonStr)
		xi, errCode := gamer.Item().AddItems(items, reason)
		ch <- addItemResult{items: xi, err: errCode}
	}); err != nil {
		xlog.Warnf("reqGamerAddItem: add item  gid:%d error:%v", req.Gid, err)
		rsp.Error = &errorpb.RpcError{ErrCode: errorpb.ERROR_RPC_GAMER_TASK_ADD_FAILED, ErrDesc: err.Error()}
		return rsp
	}

	select {
	case result := <-ch:
		if result.err != errorpb.ERROR_SUCCESS {
			rsp.GetAddItemRsp().Ok = false
			return rsp
		}
		rsp.GetAddItemRsp().Ok = true
		rsp.GetAddItemRsp().Items = result.items
	case <-time.After(gamerRpcWaitTimeout):
		xlog.Errorf("reqGamerAddItem: wait timeout gid:%d", req.Gid)
		rsp.Error = &errorpb.RpcError{ErrCode: errorpb.ERROR_RPC_SERVER_PROCESSING, ErrDesc: "wait timeout"}
	}
	return rsp
}

func (h *Handler) HandleRpcGamerSubItem(_ netmgr.IMsgQue, msg *msg.Message) *pbrpc.S2SRpcRSP {
	req := msg.Message().(*pbrpc.S2SSubItemREQ)
	rsp := &pbrpc.S2SRpcRSP{
		RspType: &pbrpc.S2SRpcRSP_SubItemRsp{SubItemRsp: &pbrpc.S2SSubItemRSP{}},
	}
	gamer := actor.FindGamerWithGid(req.Gid)
	if gamer == nil {
		xlog.Warnf("reqGamerSubItem: gamer not found gid:%v", req.Gid)
		return rsp
	}
	ch := make(chan *pbrpc.S2SSubItemRSP, 1)
	if err := gamer.AddMsgTask(pb.MSG_ID(msg.MsgId()), func() {
		reason := common.BuildReason(req.Reason, "", req.ReasonStr)
		ret, err := gamer.Item().SubItems(req.Items, reason)
		if err != errorpb.ERROR_SUCCESS {
			xlog.Warnf("reqGamerSubItem: sub item error")
			ch <- &pbrpc.S2SSubItemRSP{Ok: false}
			return
		}
		ch <- &pbrpc.S2SSubItemRSP{
			Ok:    true,
			Items: ret,
		}
	}); err != nil {
		rsp.Error = &errorpb.RpcError{ErrCode: errorpb.ERROR_RPC_SERVER_RSP_ERROR, ErrDesc: "ch is full"}
		return rsp
	}
	select {
	case subitemRsp := <-ch:
		rsp.RspType = &pbrpc.S2SRpcRSP_SubItemRsp{SubItemRsp: subitemRsp}
	case <-time.After(gamerRpcWaitTimeout):
		xlog.Errorf("reqGamerSubItem: wait timeout gid:%d", req.Gid)
		rsp.Error = &errorpb.RpcError{ErrCode: errorpb.ERROR_RPC_SERVER_RSP_ERROR, ErrDesc: "wait timeout"}
	}
	return rsp
}

func (h *Handler) HandleRpcGamerCheckItem(_ netmgr.IMsgQue, msg *msg.Message) *pbrpc.S2SRpcRSP {
	req := msg.Message().(*pbrpc.S2SCheckItemREQ)
	rsp := &pbrpc.S2SRpcRSP{
		RspType: &pbrpc.S2SRpcRSP_CheckItemRsp{CheckItemRsp: &pbrpc.S2SCheckItemRSP{}},
	}
	gamer := actor.FindGamerWithGid(req.Gid)
	if gamer == nil {
		xlog.Infof("reqGamerCheckItem: gamer not found gid:%v", req.Gid)
		return rsp
	}
	ch := make(chan bool, 1)
	if err := gamer.AddMsgTask(pb.MSG_ID(msg.MsgId()), func() {
		ok := gamer.Item().CheckEnough(req.Items)
		ch <- ok == errorpb.ERROR_SUCCESS
	}); err != nil {
		rsp.Error = &errorpb.RpcError{ErrCode: errorpb.ERROR_RPC_SERVER_RSP_ERROR, ErrDesc: err.Error()}
		return rsp
	}

	select {
	case ok := <-ch:
		rsp.GetCheckItemRsp().Ok = ok
	case <-time.After(gamerRpcWaitTimeout):
		xlog.Errorf("reqGamerCheckItem: wait timeout gid:%d", req.Gid)
		rsp.Error = &errorpb.RpcError{ErrCode: errorpb.ERROR_RPC_SERVER_RSP_ERROR, ErrDesc: "wait timeout"}
	}
	return rsp
}

func (h *Handler) HandleUseItem(ctx iface.IGamerContext, data *msg.Message) (code errorpb.ERROR, m proto.Message) {
	req := data.Message().(*pb.UseItemREQ)
	if req.Item == nil || req.Item.Num <= 0 {
		return errorpb.ERROR_REQUEST_PARAMS, nil
	}
	ctx.Logger().Debug("use item:%v", req.Item)
	req.Item.Num = 1

	packInfo := ctx.Item().UseItem(req.Item)
	if packInfo == nil {
		return errorpb.ERROR_ITEM_NOT_ENOUGH, nil
	}
	ctx.Logger().Debug("use item:%v rewards:%v", req.Item, packInfo)
	rsp := &pb.UseItemRSP{Items: packInfo}
	return errorpb.ERROR_SUCCESS, rsp
}
