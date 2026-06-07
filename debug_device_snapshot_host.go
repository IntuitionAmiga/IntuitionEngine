package main

import (
	"encoding/json"
	"fmt"
	"time"
)

const (
	fileIODeviceSnapshotVersion       = 3
	mediaLoaderSnapshotVersion        = 1
	programExecutorSnapshotVersion    = 1
	coprocessorManagerSnapshotVersion = 1
)

type fileIODeviceDebugSnapshot struct {
	FileNamePtr   uint32
	FileDataPtr   uint64
	FileDataPtr64 bool
	FileDataLen   uint32
	FileStatus    uint32
	FileResultLen uint32
	FileErrorCode uint32
}

func (f *FileIODevice) DebugSnapshotName() string { return "file-io" }

func (f *FileIODevice) DebugSnapshot() (uint32, []byte, error) {
	if f == nil {
		return fileIODeviceSnapshotVersion, nil, fmt.Errorf("nil file I/O device")
	}
	return marshalSnapshot(fileIODeviceSnapshotVersion, fileIODeviceDebugSnapshot{
		FileNamePtr: f.fileNamePtr, FileDataPtr: f.fileDataPtr, FileDataPtr64: f.fileDataPtr64, FileDataLen: f.fileDataLen,
		FileStatus: f.fileStatus, FileResultLen: f.fileResultLen, FileErrorCode: f.fileErrorCode,
	})
}

func (f *FileIODevice) DebugRestoreSnapshot(version uint32, data []byte) error {
	if f == nil {
		return fmt.Errorf("nil file I/O device")
	}
	if version != fileIODeviceSnapshotVersion && version != 2 && version != 1 {
		return fmt.Errorf("unsupported file I/O snapshot version %d", version)
	}
	var snap fileIODeviceDebugSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	f.fileNamePtr, f.fileDataPtr, f.fileDataPtr64, f.fileDataLen = snap.FileNamePtr, snap.FileDataPtr, snap.FileDataPtr64, snap.FileDataLen
	f.fileStatus, f.fileResultLen, f.fileErrorCode = snap.FileStatus, snap.FileResultLen, snap.FileErrorCode
	return nil
}

type mediaLoaderDebugSnapshot struct {
	NamePtr uint32
	Subsong uint32
	Status  uint32
	Typ     uint32
	ErrCode uint32
	ReqGen  uint64
}

func (m *MediaLoader) DebugSnapshotName() string { return "media-loader" }

func (m *MediaLoader) DebugSnapshot() (uint32, []byte, error) {
	if m == nil {
		return mediaLoaderSnapshotVersion, nil, fmt.Errorf("nil media loader")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshStatusLocked()
	return marshalSnapshot(mediaLoaderSnapshotVersion, mediaLoaderDebugSnapshot{
		NamePtr: m.namePtr, Subsong: m.subsong, Status: m.status, Typ: m.typ, ErrCode: m.errCode, ReqGen: m.reqGen,
	})
}

func (m *MediaLoader) DebugRestoreSnapshot(version uint32, data []byte) error {
	if m == nil {
		return fmt.Errorf("nil media loader")
	}
	if version != mediaLoaderSnapshotVersion {
		return fmt.Errorf("unsupported media loader snapshot version %d", version)
	}
	var snap mediaLoaderDebugSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.namePtr, m.subsong, m.status = snap.NamePtr, snap.Subsong, snap.Status
	m.typ, m.errCode, m.reqGen = snap.Typ, snap.ErrCode, snap.ReqGen
	return nil
}

type programExecutorDebugSnapshot struct {
	NamePtr        uint32
	Status         uint32
	Typ            uint32
	ErrCode        uint32
	Session        uint32
	GemdosHostRoot string
	GemdosDriveNum uint16
}

func (e *ProgramExecutor) DebugSnapshotName() string { return "program-executor" }

func (e *ProgramExecutor) DebugSnapshot() (uint32, []byte, error) {
	if e == nil {
		return programExecutorSnapshotVersion, nil, fmt.Errorf("nil program executor")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return marshalSnapshot(programExecutorSnapshotVersion, programExecutorDebugSnapshot{
		NamePtr: e.namePtr, Status: e.status, Typ: e.typ, ErrCode: e.errCode, Session: e.session,
		GemdosHostRoot: e.gemdosHostRoot, GemdosDriveNum: e.gemdosDriveNum,
	})
}

func (e *ProgramExecutor) DebugRestoreSnapshot(version uint32, data []byte) error {
	if e == nil {
		return fmt.Errorf("nil program executor")
	}
	if version != programExecutorSnapshotVersion {
		return fmt.Errorf("unsupported program executor snapshot version %d", version)
	}
	var snap programExecutorDebugSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.namePtr, e.status, e.typ, e.errCode, e.session = snap.NamePtr, snap.Status, snap.Typ, snap.ErrCode, snap.Session
	e.gemdosHostRoot, e.gemdosDriveNum = snap.GemdosHostRoot, snap.GemdosDriveNum
	return nil
}

type coprocessorCompletionDebugSnapshot struct {
	Ticket     uint32
	CPUType    uint32
	Status     uint32
	ResultCode uint32
	RespLen    uint32
	Observed   bool
	CreatedNS  int64
}

type coprocessorManagerDebugSnapshot struct {
	NextTicket           uint32
	Completions          []coprocessorCompletionDebugSnapshot
	Cmd                  uint32
	CPUType              uint32
	CmdStatus            uint32
	CmdError             uint32
	Ticket               uint32
	TicketStatus         uint32
	Op                   uint32
	ReqPtr               uint32
	ReqLen               uint32
	RespPtr              uint32
	RespCap              uint32
	Timeout              uint32
	NamePtr              uint32
	WorkerState          uint32
	OpsDispatched        uint32
	BytesProcessed       uint64
	CompletionIRQEnabled bool
	CompletedTicket      uint32
	DispatchOverheadNS   uint64
}

func (m *CoprocessorManager) DebugSnapshotName() string { return "coprocessor-manager" }

func (m *CoprocessorManager) DebugSnapshot() (uint32, []byte, error) {
	if m == nil {
		return coprocessorManagerSnapshotVersion, nil, fmt.Errorf("nil coprocessor manager")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	completions := make([]coprocessorCompletionDebugSnapshot, 0, len(m.completions))
	for _, c := range m.completions {
		if c == nil {
			continue
		}
		createdNS := int64(0)
		if !c.created.IsZero() {
			createdNS = c.created.UnixNano()
		}
		completions = append(completions, coprocessorCompletionDebugSnapshot{
			Ticket: c.ticket, CPUType: c.cpuType, Status: c.status, ResultCode: c.resultCode,
			RespLen: c.respLen, Observed: c.observed, CreatedNS: createdNS,
		})
	}
	return marshalSnapshot(coprocessorManagerSnapshotVersion, coprocessorManagerDebugSnapshot{
		NextTicket: m.nextTicket, Completions: completions, Cmd: m.cmd, CPUType: m.cpuType,
		CmdStatus: m.cmdStatus, CmdError: m.cmdError, Ticket: m.ticket, TicketStatus: m.ticketStatus,
		Op: m.op, ReqPtr: m.reqPtr, ReqLen: m.reqLen, RespPtr: m.respPtr, RespCap: m.respCap,
		Timeout: m.timeout, NamePtr: m.namePtr, WorkerState: m.workerState,
		OpsDispatched: m.opsDispatched, BytesProcessed: m.bytesProcessed,
		CompletionIRQEnabled: m.completionIRQEnabled.Load(), CompletedTicket: m.completedTicket.Load(),
		DispatchOverheadNS: m.dispatchOverheadNs.Load(),
	})
}

func (m *CoprocessorManager) DebugRestoreSnapshot(version uint32, data []byte) error {
	if m == nil {
		return fmt.Errorf("nil coprocessor manager")
	}
	if version != coprocessorManagerSnapshotVersion {
		return fmt.Errorf("unsupported coprocessor manager snapshot version %d", version)
	}
	var snap coprocessorManagerDebugSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	completions := make(map[uint32]*CoprocCompletion, len(snap.Completions))
	for _, c := range snap.Completions {
		created := time.Time{}
		if c.CreatedNS != 0 {
			created = time.Unix(0, c.CreatedNS)
		}
		completions[c.Ticket] = &CoprocCompletion{
			ticket: c.Ticket, cpuType: c.CPUType, status: c.Status, resultCode: c.ResultCode,
			respLen: c.RespLen, observed: c.Observed, created: created,
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextTicket, m.completions = snap.NextTicket, completions
	m.cmd, m.cpuType, m.cmdStatus, m.cmdError = snap.Cmd, snap.CPUType, snap.CmdStatus, snap.CmdError
	m.ticket, m.ticketStatus, m.op = snap.Ticket, snap.TicketStatus, snap.Op
	m.reqPtr, m.reqLen, m.respPtr, m.respCap = snap.ReqPtr, snap.ReqLen, snap.RespPtr, snap.RespCap
	m.timeout, m.namePtr, m.workerState = snap.Timeout, snap.NamePtr, snap.WorkerState
	m.opsDispatched, m.bytesProcessed = snap.OpsDispatched, snap.BytesProcessed
	m.completionIRQEnabled.Store(snap.CompletionIRQEnabled)
	m.completedTicket.Store(snap.CompletedTicket)
	m.dispatchOverheadNs.Store(snap.DispatchOverheadNS)
	return nil
}
