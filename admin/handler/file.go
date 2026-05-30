package handler

import (
	"TengShe/admin/manager"
	"TengShe/protocol"
	"TengShe/share"
)

func StartBar(statusChan chan *share.Status, size int64) {
	bar := NewBar(size)

	for {
		status := <-statusChan
		switch status.Stat {
		case share.START:
			bar.Start()
		case share.ADD:
			bar.Add64(status.Scale)
		case share.DONE:
			bar.Finish()
			return
		}
	}
}

func DispatchFileMess(mgr *manager.Manager) {
	for {
		message := <-mgr.FileManager.FileMessChan

		switch mess := message.(type) {
		case *protocol.FileStatReq:
			mgr.FileManager.File.FileSize = int64(mess.FileSize)
			mgr.FileManager.File.SliceNum = mess.SliceNum
			mgr.ConsoleManager.OK <- true
		case *protocol.FileStatRes:
			if mess.OK == 1 {
				mgr.ConsoleManager.OK <- true
			} else {
				mgr.FileManager.File.Handler.Close()
				mgr.ConsoleManager.OK <- false
			}
		case *protocol.FileDownRes:
			mgr.ConsoleManager.OK <- false
		case *protocol.FileData:
			mgr.FileManager.File.DataChan <- mess.Data
		case *protocol.FileErr:
			mgr.FileManager.File.ErrChan <- true
		}
	}
}
