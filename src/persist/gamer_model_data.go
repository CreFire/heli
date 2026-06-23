package persist

import "game/src/proto/pb"

func newGamerShopData() *pb.GamerShopData {
	return &pb.GamerShopData{
		ShopTabs: make(map[int32]*pb.ShopTabData),
	}
}

func newGamerPlayerData() *pb.PlayerBase {
	return &pb.PlayerBase{}
}

func newGamerMainData() *pb.Gamer {
	return &pb.Gamer{}
}

func newGamerPackData() *pb.GamerPackData {
	return &pb.GamerPackData{
		ItemMap: make(map[string]*pb.Item, 16),
	}
}

func newGamerTaskData() *pb.GamerTaskData {
	return &pb.GamerTaskData{
		TaskMap: make(map[int32]*pb.GameTask, 16),
	}
}

func newGamerRecordData() *pb.GamerRecordData {
	return &pb.GamerRecordData{
		RecordMap: make(map[int32]int64, 0),
	}
}

// func newGamerFunctionData() *pb.GamerFunctionOpen {
// 	return &pb.GamerFunctionOpen{Functions: make(map[int32]bool)}
// }
