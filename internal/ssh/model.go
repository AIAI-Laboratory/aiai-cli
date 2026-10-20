package ssh

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	ErrValidation = errors.New("invalid SSH configuration")
	ErrNotFound   = errors.New("SSH server not found")
	ErrDuplicate  = errors.New("SSH server alias already exists")
	ErrStorage    = errors.New("SSH storage failure")
	ErrSSHMissing = errors.New("OpenSSH client not found")
)

const (
	StatusPending  = "pending"
	StatusSuccess  = "success"
	StatusFailed   = "failed"
	StatusCanceled = "canceled"
)

type Server struct {
	Alias        string    `json:"alias"`
	Host         string    `json:"host"`
	Port         int       `json:"port"`
	Username     string    `json:"username"`
	IdentityFile string    `json:"identity_file,omitempty"`
	Description  string    `json:"description,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type AddRequest struct {
	Alias        string
	Host         string
	Port         int
	Username     string
	IdentityFile string
	Description  string
}

type UpdateRequest struct {
	Host         *string
	Port         *int
	Username     *string
	IdentityFile *string
	Description  *string
}

type AccessLog struct {
	ID          string     `json:"id"`
	ServerAlias string     `json:"server_alias"`
	Host        string     `json:"host"`
	Username    string     `json:"username"`
	StartedAt   time.Time  `json:"started_at"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
	DurationMS  *int64     `json:"duration_ms,omitempty"`
	ExitCode    *int       `json:"exit_code,omitempty"`
	Status      string     `json:"status"`
}

type LogFilter struct {
	ServerAlias string
	Limit       int
}

type Repository interface {
	AddServer(context.Context, Server) error
	ListServers(context.Context) ([]Server, error)
	GetServer(context.Context, string) (Server, error)
	UpdateServer(context.Context, Server) error
	RemoveServer(context.Context, string) error
	StartLog(context.Context, AccessLog) error
	FinishLog(context.Context, AccessLog) error
	ListLogs(context.Context, LogFilter) ([]AccessLog, error)
}

type Streams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

type RunResult struct {
	ExitCode int
}

type Key struct {
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Fingerprint string    `json:"fingerprint"`
	Comment     string    `json:"comment,omitempty"`
	PrivatePath string    `json:"private_path"`
	PublicPath  string    `json:"public_path"`
	Managed     bool      `json:"managed"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
}

type KeyGenerateRequest struct {
	Name         string
	Type         string
	Bits         int
	Comment      string
	NoPassphrase bool
}

type Runner interface {
	Run(context.Context, []string, Streams) (RunResult, error)
}

type Manager interface {
	Add(context.Context, AddRequest) (Server, error)
	List(context.Context) ([]Server, error)
	Show(context.Context, string) (Server, error)
	Update(context.Context, string, UpdateRequest) (Server, error)
	Remove(context.Context, string) error
	SetKey(context.Context, string, string) (Server, error)
	ShowKey(context.Context, string) (string, error)
	ClearKey(context.Context, string) (Server, error)
	Connect(context.Context, string, Streams) error
	Logs(context.Context, LogFilter) ([]AccessLog, error)
	GenerateKey(context.Context, KeyGenerateRequest, Streams) (Key, error)
	FindKeys(context.Context, string) ([]Key, error)
	FindKey(context.Context, string) (Key, error)
	PublicKey(context.Context, string) (string, error)
	RemoveKey(context.Context, string) error
}

type ExitError struct {
	Code int
}

func (e ExitError) Error() string { return "ssh exited with status " + itoa(e.Code) }
func (e ExitError) ExitCode() int { return e.Code }

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var buf [20]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
