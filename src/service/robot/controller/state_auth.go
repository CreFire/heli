package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"game/deps/msg"
	"game/deps/netmgr/options"
	"game/deps/xlog"
	"game/src/proto/errorpb"
	"game/src/proto/pb"
	"io"
	"net/http"
	"net/netip"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
)

const authUseRoleQueueWaitTimeout = 60 * time.Second

type StateAuth struct {
	authLoginRsp   *pb.AuthLoginRSP   // 缓存鉴权登录返回，用于后续选角和建连。
	authUseRoleRsp *pb.AuthUseRoleRSP // 缓存选角返回，用于连接网关。
}

func (s *StateAuth) Name() string {
	return STATE_AUTH
}

func (s *StateAuth) OnEnter(robot *Robot) error {
	err := s.authLogin(robot)
	if err != nil {
		return err
	}

	err = s.authUseRole(robot)
	if err != nil {
		return err
	}
	err = s.connectGate(robot)
	if err != nil {
		return err
	}
	return nil
}

func (s *StateAuth) onLeave(robot *Robot) {

}

func (s *StateAuth) onUpdate(robot *Robot) {

}

func (s *StateAuth) register(robot *Robot) {
}

func (s *StateAuth) authLogin(robot *Robot) error {
	reqData := &pb.AuthLoginREQ{
		Account: robot.name,
		Token:   "admin123.",
		DeviceInfo: &pb.DeviceInfo{
			DeviceId: fmt.Sprintf("tttt-%s", robot.name),
		},
	}
	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%v/api/v1/auth/login", robotServerCfg().Robot.Auth)
	resp, err := RobotSvr.client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("robot[%s] auth failed. : %v", robot.name, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	s.authLoginRsp = &pb.AuthLoginRSP{}
	err = protojson.Unmarshal(body, s.authLoginRsp)
	if err != nil {
		return err
	}
	if s.authLoginRsp.Err != nil {
		xlog.Errorf("robot[%s] auth failed. : %v", robot.name, s.authLoginRsp.Err)
		return err
	}
	if len(s.authLoginRsp.Roles) == 0 || s.authLoginRsp.Roles[0] == nil {
		xlog.Errorf("robot[%s] auth failed: empty roles", robot.name)
		return err
	}
	robot.gid = s.authLoginRsp.Roles[0].Gid
	return nil
}
func (s *StateAuth) authUseRole(robot *Robot) error {
	reqData := &pb.AuthUseRoleREQ{
		Session:    s.authLoginRsp.Session,
		DeviceInfo: &pb.DeviceInfo{Gid: robot.gid, DeviceId: robot.name},
		Name:       robot.name,
	}
	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%v/api/v1/auth/userole", robotServerCfg().Robot.Auth)
	deadline := time.Now().Add(authUseRoleQueueWaitTimeout)
	for {
		rsp, err := s.doAuthUseRole(robot, url, jsonData)
		if err != nil {
			return err
		}
		s.authUseRoleRsp = rsp
		if rsp.Err != nil {
			return fmt.Errorf("robot[%s] auth failed. : %v", robot.name, rsp.Err)
		}
		if rsp.Queue == nil {
			robot.gid = rsp.Gid
			robot.Session = s.authLoginRsp.Session
			return nil
		}

		now := time.Now()
		if !now.Before(deadline) {
			return fmt.Errorf("robot[%s] auth use role queue timeout. gid:%d index:%d nextReqTime:%d", robot.name, robot.gid, rsp.Queue.Index, rsp.Queue.NextReqTime)
		}
		wait := nextAuthUseRoleQueueWait(rsp.Queue, now)
		remaining := time.Until(deadline)
		if wait > remaining {
			wait = remaining
		}
		xlog.Infof("robot[%s] userole queued gid:%d index:%d wait:%s nextReqTime:%d", robot.name, robot.gid, rsp.Queue.Index, wait, rsp.Queue.NextReqTime)
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-robot.quit:
			timer.Stop()
			return fmt.Errorf("robot[%s] stopped while waiting auth queue", robot.name)
		case <-robotRuntimeDone():
			timer.Stop()
			return fmt.Errorf("robot[%s] runtime stopped while waiting auth queue", robot.name)
		}
	}
}

func (s *StateAuth) connectGate(robot *Robot) error {
	addr, err := resolveAuthUseRoleGateAddr(s.authUseRoleRsp)
	if err != nil {
		return err
	}
	opt := options.NewMsgQueOptions()
	opt.SetReadSize(20 * 1024).SetWriteSize(20 * 1024).SetWriteChanSize(options.WRITE_CHAN_SIZE_C)
	opt.SetConnectParams(options.NewConnectParams(addr, "gate", 100))
	opt.SetEnableDH(true)
	err = robotNetMgr().StartConnect(opt, robot.msgHandler)
	if err != nil {
		return err
	}
	return nil
}

func (s *StateAuth) doAuthUseRole(robot *Robot, url string, jsonData []byte) (*pb.AuthUseRoleRSP, error) {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", s.authLoginRsp.Session)

	resp, err := RobotSvr.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("robot[%s] auth failed. : %v", robot.name, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	rsp := &pb.AuthUseRoleRSP{}
	if err := protojson.Unmarshal(body, rsp); err != nil {
		return nil, err
	}
	return rsp, nil
}

func nextAuthUseRoleQueueWait(queue *pb.UserLoginQueue, now time.Time) time.Duration {
	if queue == nil {
		return time.Second
	}
	if queue.NextReqTime > 0 {
		waitUntil := time.Unix(queue.NextReqTime, 0)
		if d := time.Until(waitUntil); d > 0 {
			return d
		}
	}
	if queue.NextDuration > 0 {
		return time.Duration(queue.NextDuration) * time.Second
	}
	return time.Second
}

func resolveAuthUseRoleGateAddr(rsp *pb.AuthUseRoleRSP) (string, error) {
	if rsp == nil {
		return "", fmt.Errorf("auth use role response is nil")
	}
	if rsp.Host != "" {
		return rsp.Host, nil
	}
	if rsp.Addr == nil || rsp.Addr.Ip == "" || rsp.Addr.Port <= 0 {
		return "", fmt.Errorf("auth use role response missing gate endpoint")
	}
	addr, err := netip.ParseAddr(rsp.Addr.Ip)
	if err == nil && addr.Is6() {
		return netip.AddrPortFrom(addr, uint16(rsp.Addr.Port)).String(), nil
	}
	return fmt.Sprintf("%s:%d", rsp.Addr.Ip, rsp.Addr.Port), nil
}

type StateLogin struct {
}

func (s *StateLogin) Name() string {
	return STATE_LOGIN
}

func (s *StateLogin) OnEnter(robot *Robot) error {
	robot.Send(&pb.LoginBySessionREQ{Gid: robot.gid, Session: robot.Session, DeviceInfo: &pb.DeviceInfo{DeviceId: robot.name}})
	xlog.Debugf("start login")
	return nil
}

func (s *StateLogin) onLeave(robot *Robot) {

}

func (s *StateLogin) onUpdate(robot *Robot) {
}

func (s *StateLogin) register(robot *Robot) {
	robot.msgHandler.Register(pb.MSG_ID_LOGIN_BY_SESSION_REQ, pb.MSG_ID_LOGIN_BY_SESSION_RSP, &pb.LoginBySessionREQ{}, &pb.LoginBySessionRSP{}, reqLogin)
}

func reqLogin(robot *Robot, message *msg.Message) {
	recv, _ := message.Message().(*pb.LoginBySessionRSP)
	if recv == nil || recv.Err != errorpb.ERROR_SUCCESS {
		WarnfLimited("login_failed", "robot[%s] login failed.", robot.name)
		robot.Stop()
		return
	}
	main := recv.GetMain()
	if main != nil {
		robot.recordSmokeResult("LOGIN", "main", true, fmt.Sprintf("id=%d account=%s", main.GetId(), main.GetAccount()))
	} else {
		robot.recordSmokeResult("LOGIN", "main", false, "main=nil")
	}
	xlog.Debugf("robot[%s] login success.", robot.name)
	RobotSvr.robotMgr.addRobot(robot.msgque.SessId(), robot.gid, robot)
	robot.SetState(STATE_GM)
	return
}
