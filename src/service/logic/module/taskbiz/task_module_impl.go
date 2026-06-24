package taskbiz

import (
	"sort"
	"strconv"

	"game/src/common"
	"game/src/configdoc"
	docpb "game/src/proto/docpb"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"game/src/service/logic/gamedata"
	"game/src/service/logic/iface"

	"google.golang.org/protobuf/proto"
)

type TaskModule struct {
	ctx   iface.IGamerContext
	model *gamedata.GamerModel
	store *GamerTask
}

func NewTaskModule(ctx iface.IGamerContext, model *gamedata.GamerModel) *TaskModule {
	return &TaskModule{ctx: ctx, model: model, store: GetTaskModel(model)}
}

func (m *TaskModule) SendTaskInfo() {
	m.ensureOpenTasks()
	m.ctx.SendMsg(&pb.TaskInfoNTF{
		IsAll: true,
		Tasks: m.sortedTasks(),
		Data:  m.store.Data(),
	})
}

func (m *TaskModule) SendTaskSync() {
	m.ctx.SendMsg(&pb.TaskChangeRSP{Tasks: m.sortedTasks()})
}

func (m *TaskModule) CheckTaskCompleted(id int32) bool {
	task := m.taskMap()[id]
	return task != nil && task.State == pb.TASK_STATE_TASK_FINISH
}

func (m *TaskModule) Reward(req *pb.TaskRewardREQ) (errorpb.ERROR, proto.Message) {
	if req == nil || req.TaskId <= 0 {
		return errorpb.ERROR_REQUEST_PARAMS, nil
	}
	m.ensureOpenTasks()
	task := m.taskMap()[req.TaskId]
	if task == nil {
		return errorpb.ERROR_FAILED, nil
	}
	if task.State == pb.TASK_STATE_TASK_OVER {
		return errorpb.ERROR_FAILED, nil
	}
	if task.State != pb.TASK_STATE_TASK_FINISH {
		return errorpb.ERROR_FAILED, nil
	}

	conf := m.taskConf(req.TaskId)
	if conf == nil {
		return errorpb.ERROR_FAILED, nil
	}

	rewards := make([]*pb.Item, 0, 1)
	if conf.RewardPoints > 0 {
		rewards = append(rewards, &pb.Item{ConfId: strconv.Itoa(configdoc.COIN_GOLD), Num: int64(conf.RewardPoints)})
		if m.ctx.Item() == nil {
			return errorpb.ERROR_FAILED, nil
		}
		if _, code := m.ctx.Item().AddItems(rewards, common.BuildReason(common.ReasonTask.Id, "", "task reward")); code != errorpb.ERROR_SUCCESS {
			return code, nil
		}
	}

	task.State = pb.TASK_STATE_TASK_OVER
	m.store.AddUpdateOp("task_map", m.store.Data().TaskMap)
	return errorpb.ERROR_SUCCESS, &pb.TaskRewardRSP{Items: rewards}
}

func (m *TaskModule) Refresh(req *pb.TaskRefreshREQ) (errorpb.ERROR, proto.Message) {
	if req == nil {
		return errorpb.ERROR_REQUEST_PARAMS, nil
	}
	m.refreshOpenTasks()
	return errorpb.ERROR_SUCCESS, &pb.TaskRefreshRSP{}
}

func (m *TaskModule) ensureOpenTasks() {
	if m == nil || m.store == nil {
		return
	}
	if len(m.taskMap()) > 0 {
		return
	}
	m.refreshOpenTasks()
}

func (m *TaskModule) refreshOpenTasks() {
	if m == nil || m.store == nil || m.ctx == nil || m.ctx.Doc() == nil || m.ctx.Doc().TbDailyTask == nil {
		return
	}
	taskMap := m.taskMap()
	for _, conf := range m.ctx.Doc().TbDailyTask.GetDataList() {
		if conf == nil || conf.IsOpen == 0 {
			continue
		}
		if _, ok := taskMap[conf.Id]; ok {
			continue
		}
		state := pb.TASK_STATE_TASK_DOING
		if conf.CompleteValue <= 0 {
			state = pb.TASK_STATE_TASK_FINISH
		}
		taskMap[conf.Id] = &pb.GameTask{
			Id:        conf.Id,
			State:     state,
			Progress:  conf.CompleteValue,
			FinishNum: conf.CompleteValue,
			StartTime: m.ctx.Now(),
			LastTime:  m.ctx.Now(),
			TaskType:  conf.KeyId,
		}
	}
	m.store.AddUpdateOp("task_map", m.store.Data().TaskMap)
}

func (m *TaskModule) sortedTasks() []*pb.GameTask {
	taskMap := m.taskMap()
	ids := make([]int32, 0, len(taskMap))
	for id := range taskMap {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]*pb.GameTask, 0, len(ids))
	for _, id := range ids {
		out = append(out, proto.Clone(taskMap[id]).(*pb.GameTask))
	}
	return out
}

func (m *TaskModule) taskMap() map[int32]*pb.GameTask {
	return m.store.EnsureTaskMap()
}

func (m *TaskModule) taskConf(id int32) *docpb.Daily_taskDailyTask {
	if m.ctx == nil || m.ctx.Doc() == nil || m.ctx.Doc().TbDailyTask == nil {
		return nil
	}
	return m.ctx.Doc().TbDailyTask.Get(id)
}
