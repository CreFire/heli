package task

import (
	"sort"

	"game/deps/xtime"
	"game/src/common"
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

	rewards, ok := m.buildRewardItems(conf.RewardItem)
	if !ok {
		return errorpb.ERROR_FAILED, nil
	}
	if len(rewards) > 0 {
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
	if m == nil || m.store == nil || m.ctx == nil || m.ctx.Doc() == nil || m.ctx.Doc().TbTask == nil {
		return
	}
	taskMap := m.taskMap()
	now := m.ctx.Now()
	m.resetExpiredTasks(taskMap, now)
	for _, conf := range m.ctx.Doc().TbTask.GetDataList() {
		if !m.canOpenTask(conf) {
			continue
		}
		if _, ok := taskMap[conf.TaskID]; ok {
			continue
		}
		taskMap[conf.TaskID] = &pb.GameTask{
			Id:        conf.TaskID,
			State:     pb.TASK_STATE_TASK_DOING,
			Progress:  0,
			FinishNum: 0,
			StartTime: now,
			LastTime:  now,
			TaskType:  conf.TaskType,
		}
	}
	m.store.AddUpdateOp("task_map", m.store.Data().TaskMap)
}

func (m *TaskModule) resetExpiredTasks(taskMap map[int32]*pb.GameTask, now int64) {
	for taskID, task := range taskMap {
		conf := m.taskConf(taskID)
		if conf == nil {
			delete(taskMap, taskID)
			continue
		}
		if !m.needReset(conf.TaskResetType, task.StartTime, now) {
			continue
		}
		delete(taskMap, taskID)
	}
}

func (m *TaskModule) needReset(resetType int32, startTime, now int64) bool {
	switch resetType {
	case common.TASK_RESET_DAILY:
		return startTime > 0 && xtime.CheckDailyFresh(startTime, now)
	case common.TASK_RESET_WEEKLY:
		return startTime > 0 && xtime.CheckWeeklyFresh(startTime, now)
	default:
		return false
	}
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

func (m *TaskModule) taskConf(id int32) *docpb.Task {
	if m.ctx == nil || m.ctx.Doc() == nil || m.ctx.Doc().TbTask == nil {
		return nil
	}
	return m.ctx.Doc().TbTask.Get(id)
}

func (m *TaskModule) canOpenTask(conf *docpb.Task) bool {
	if conf == nil || conf.TaskID <= 0 {
		return false
	}
	if conf.TaskUnlock <= 0 {
		return true
	}
	prev := m.taskMap()[conf.TaskUnlock]
	return prev != nil && prev.State == pb.TASK_STATE_TASK_OVER
}

func (m *TaskModule) buildRewardItems(raw []int32) ([]*pb.Item, bool) {
	if len(raw) == 0 {
		return nil, true
	}
	if len(raw)%2 != 0 {
		return nil, false
	}
	items := make([]*pb.Item, 0, len(raw)/2)
	for i := 0; i < len(raw); i += 2 {
		confID := raw[i]
		num := raw[i+1]
		if confID <= 0 || num <= 0 {
			return nil, false
		}
		items = append(items, &pb.Item{ConfId: confID, Num: int64(num)})
	}
	return items, true
}
