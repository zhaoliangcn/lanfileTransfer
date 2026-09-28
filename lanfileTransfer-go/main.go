package main

import (
	"LanFileTransfer-Go/internal/config"
	"LanFileTransfer-Go/internal/discovery"
	"LanFileTransfer-Go/internal/file"
	"LanFileTransfer-Go/internal/network"
	"LanFileTransfer-Go/internal/service"
	"LanFileTransfer-Go/internal/transfer"
	"LanFileTransfer-Go/pkg/utils"
	"context"
	"embed"
	"fmt"
	"os"
	"os/exec"
	golangruntime "runtime"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

type App struct {
	ctx             context.Context
	config          *config.Config
	transferMgr     *transfer.TransferManager
	discoveryMgr    *discovery.DiscoveryManager
	fileService     *service.FileService
	peerService     *service.PeerService
	transferService *service.TransferService
	deviceID        string
	deviceName      string
	pendingRestart  []string
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	utils.InitLogger("info")
	utils.SugaredLog.Infow("starting LanFileTransfer application")

	a.deviceID = a.getOrCreateDeviceID()
	hostname, _ := os.Hostname()
	a.deviceName = fmt.Sprintf("%s (%s)", hostname, "LanFileTransfer")

	a.config = config.Load("")

	// The configured save path used to be written to disk but never read, so
	// inbound transfers always landed in the hard-coded default.
	if a.config.DefaultSavePath != "" {
		resolved := file.SetDefaultSavePath(a.config.DefaultSavePath)
		utils.SugaredLog.Infow("save path resolved", "path", resolved)
	}

	configDir, _ := os.UserConfigDir()
	historyMgr := transfer.NewHistoryManager(configDir, 500)
	historyMgr.Load()

	a.discoveryMgr = discovery.NewDiscoveryManager(a.deviceID, a.deviceName, a.config.ListenPort, a.config.DiscoveryInterval)
	a.transferMgr = transfer.NewTransferManager(a.config, historyMgr)

	a.fileService = service.NewFileService()
	a.peerService = service.NewPeerService(a.discoveryMgr)
	a.transferService = service.NewTransferService(a.transferMgr)

	a.transferMgr.OnEvent(func(event *transfer.TransferEvent) {
		runtime.EventsEmit(a.ctx, "transferEvent", event)
	})

	a.discoveryMgr.OnPeerFound(func(peer *discovery.Peer) {
		runtime.EventsEmit(a.ctx, "peerFound", peer)
	})

	a.discoveryMgr.OnPeerLost(func(peerID string) {
		runtime.EventsEmit(a.ctx, "peerLost", peerID)
	})

	a.discoveryMgr.OnChatMessage(func(msg *network.ControlMessage) {
		runtime.EventsEmit(a.ctx, "chatMessage", map[string]interface{}{
			"senderId":   msg.DeviceID,
			"senderName": msg.DeviceName,
			"message":    msg.Message,
			"timestamp":  msg.Timestamp,
		})
	})

	a.discoveryMgr.OnSystemCommand(func(msg *network.ControlMessage) {
		utils.SugaredLog.Warnw("received system command",
			"sender", msg.DeviceID,
			"command", msg.Command,
		)

		// The dialog below is only a speed bump, not a security control: anyone on
		// the LAN can craft the UDP datagram. A device_id is trivially guessable
		// ("<hostname>-<unixtime>" on the C side, a plain file on this side), so
		// require the shared secret before even prompting.
		if !a.discoveryMgr.VerifyAuthToken(msg.AuthToken) {
			utils.SugaredLog.Warnw("rejected system command with invalid auth token",
				"sender", msg.DeviceID,
				"command", msg.Command,
			)
			return
		}

		// Show confirmation dialog
		cmdLabel := "shutdown"
		if msg.Command == network.ControlCommandRestart {
			cmdLabel = "restart"
		}

		dialogTitle := "Remote " + cmdLabel
		dialogMsg := fmt.Sprintf("Peer %s (%s) requested to %s this computer in 10 seconds.", msg.DeviceName, msg.DeviceID, cmdLabel)

		_, err := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type:          runtime.WarningDialog,
			Title:         dialogTitle,
			Message:       dialogMsg,
			DefaultButton: "OK",
		})

		if err != nil {
			utils.SugaredLog.Warnw("system command cancelled by user")
			return
		}

		utils.SugaredLog.Warnw("executing system command", "command", msg.Command)

		var cmd *exec.Cmd
		switch msg.Command {
		case network.ControlCommandShutdown:
			switch golangruntime.GOOS {
			case "darwin":
				cmd = exec.Command("osascript", "-e", `tell app "System Events" to shut down`)
			case "windows":
				cmd = exec.Command("shutdown", "/s", "/t", "0")
			default:
				cmd = exec.Command("shutdown", "-h", "+0")
			}
		case network.ControlCommandRestart:
			switch golangruntime.GOOS {
			case "darwin":
				cmd = exec.Command("osascript", "-e", `tell app "System Events" to restart`)
			case "windows":
				cmd = exec.Command("shutdown", "/r", "/t", "0")
			default:
				cmd = exec.Command("shutdown", "-r", "+0")
			}
		}

		if cmd != nil {
			if err := cmd.Start(); err != nil {
				utils.SugaredLog.Errorw("failed to execute system command", "error", err)
			}
		}
	})

	if err := a.discoveryMgr.Start(); err != nil {
		utils.SugaredLog.Errorw("failed to start discovery manager", "error", err)
	}

	if err := a.transferMgr.Start(); err != nil {
		utils.SugaredLog.Errorw("failed to start transfer manager", "error", err)
	}

	utils.SugaredLog.Infow("application started",
		"deviceID", a.deviceID,
		"deviceName", a.deviceName,
		"port", a.config.ListenPort,
	)
}

func (a *App) shutdown(ctx context.Context) {
	utils.SugaredLog.Infow("shutting down application")

	a.transferMgr.Stop()
	a.discoveryMgr.Stop()

	utils.SyncLogger()
}

func (a *App) getOrCreateDeviceID() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return uuid.New().String()
	}

	idPath := fmt.Sprintf("%s/LanFileTransfer/device_id", configDir)
	data, err := os.ReadFile(idPath)
	if err == nil && len(data) > 0 {
		return string(data)
	}

	id := uuid.New().String()
	os.MkdirAll(fmt.Sprintf("%s/LanFileTransfer", configDir), 0755)
	os.WriteFile(idPath, []byte(id), 0644)

	return id
}

func (a *App) GetPeers() []*discovery.Peer {
	return a.peerService.GetPeers()
}

func (a *App) RefreshPeers() []*discovery.Peer {
	return a.peerService.RefreshPeers()
}

func (a *App) GetOnlinePeers() []*discovery.Peer {
	return a.peerService.GetOnlinePeers()
}

func (a *App) GetLocalPeer() *discovery.Peer {
	return a.peerService.GetLocalPeer()
}

// AddManualPeer lets the user register a peer by address. Automatic discovery
// uses UDP broadcast, which routers do not forward, so machines on separate
// subnets or across a tunnel can only be reached this way.
func (a *App) AddManualPeer(name, address string, port int) (*discovery.Peer, error) {
	return a.discoveryMgr.AddManualPeer(name, address, port)
}

func (a *App) RemoveManualPeer(peerID string) bool {
	return a.discoveryMgr.RemoveManualPeer(peerID)
}

// ProbePeer checks whether a transfer port answers, so the UI can validate a
// manually entered address before committing to it.
func (a *App) ProbePeer(address string, port int) error {
	return a.discoveryMgr.ProbePeer(address, port)
}

// GetAuthToken returns the shared secret used to authorise remote shutdown and
// restart. Both peers must hold the same value.
func (a *App) GetAuthToken() string {
	return a.discoveryMgr.AuthToken()
}

func (a *App) SendFile(peerID, peerName, peerAddr, filePath string) (string, error) {
	return a.transferService.SendFile(peerID, peerName, peerAddr, filePath)
}

func (a *App) SendFolder(peerID, peerName, peerAddr, folderPath string) ([]string, error) {
	return a.transferService.SendFolder(peerID, peerName, peerAddr, folderPath)
}

func (a *App) ResumeTransfer(transferID string) error {
	return a.transferService.ResumeTransfer(transferID)
}

func (a *App) ResumeFailedTransfers() (int, error) {
	return a.transferService.ResumeFailedTransfers()
}

func (a *App) CancelTransfer(transferID string) error {
	return a.transferService.CancelTransfer(transferID)
}

func (a *App) GetTask(transferID string) *transfer.TransferTask {
	return a.transferService.GetTask(transferID)
}

func (a *App) GetAllTasks() []*transfer.TransferTask {
	return a.transferService.GetAllTasks()
}

func (a *App) GetActiveTasks() []*transfer.TransferTask {
	return a.transferService.GetActiveTasks()
}

func (a *App) GetFailedTasks() []*transfer.TransferTask {
	return a.transferService.GetFailedTasks()
}

func (a *App) GetTaskCount() int {
	return a.transferService.GetTaskCount()
}

func (a *App) GetFailedCount() int {
	return a.transferService.GetFailedCount()
}

func (a *App) GetTransferHistory() []*transfer.TransferTask {
	if a.transferMgr.HistoryManager() == nil {
		return nil
	}
	return a.transferMgr.HistoryManager().GetAll()
}

func (a *App) ClearTransferHistory() error {
	if a.transferMgr.HistoryManager() == nil {
		return nil
	}
	return a.transferMgr.HistoryManager().ClearAll()
}

func (a *App) IsFolder(path string) bool {
	return a.transferService.IsFolder(path)
}

func (a *App) GetFolderContents(folderPath string) ([]*file.FileInfo, error) {
	return a.transferService.GetFolderContents(folderPath)
}

func (a *App) GetConfig() *config.Config {
	return a.config
}

// SaveConfig persists the configuration and applies the parts that can take
// effect at runtime.
//
// ListenPort and DiscoveryInterval are baked into the running discovery and
// transfer servers, so they only change on restart. That set is reported back
// to the UI instead of leaving the user with a silent "Saved!".
func (a *App) SaveConfig(newConfig config.Config) error {
	pendingRestart := []string{}

	if newConfig.ListenPort != a.config.ListenPort {
		pendingRestart = append(pendingRestart, "listenPort")
	}
	if newConfig.DiscoveryInterval != a.config.DiscoveryInterval {
		pendingRestart = append(pendingRestart, "discoveryInterval")
	}

	a.config.DefaultSavePath = newConfig.DefaultSavePath
	a.config.AutoReceive = newConfig.AutoReceive
	a.config.MaxConcurrentTransfers = newConfig.MaxConcurrentTransfers
	a.config.ChunkSize = newConfig.ChunkSize
	a.config.ListenPort = newConfig.ListenPort
	a.config.DiscoveryInterval = newConfig.DiscoveryInterval

	if err := config.Save(a.config, ""); err != nil {
		utils.SugaredLog.Errorw("failed to save config", "error", err)
		return err
	}

	if a.config.DefaultSavePath != "" {
		resolved := file.SetDefaultSavePath(a.config.DefaultSavePath)
		utils.SugaredLog.Infow("save path updated", "path", resolved)
	}
	if a.transferMgr != nil {
		a.transferMgr.SetChunkSize(a.config.ChunkSize)
		a.transferMgr.SetMaxConcurrent(a.config.MaxConcurrentTransfers)
		a.transferMgr.SetAutoReceive(a.config.AutoReceive)
	}

	utils.SugaredLog.Infow("configuration saved",
		"port", a.config.ListenPort,
		"chunkSize", a.config.ChunkSize,
		"pendingRestart", pendingRestart,
	)

	a.pendingRestart = pendingRestart
	return nil
}

// PendingRestartFields lists the config keys that were saved but only apply
// after the app restarts.
func (a *App) PendingRestartFields() []string {
	return a.pendingRestart
}

func (a *App) SelectFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择文件",
	})
}

func (a *App) SelectDirectory() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择目录",
	})
}

func (a *App) SelectSaveFile(fileName string) (string, error) {
	return runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "保存文件",
		DefaultFilename: fileName,
	})
}

func (a *App) SendMessage(peerID, content string) error {
	return a.discoveryMgr.SendChatMessage(peerID, content)
}

func (a *App) RemoteShutdown(peerID string) error {
	utils.SugaredLog.Infow("sending remote shutdown", "peerID", peerID)
	return a.discoveryMgr.SendSystemCommand(peerID, network.ControlCommandShutdown)
}

func (a *App) RemoteRestart(peerID string) error {
	utils.SugaredLog.Infow("sending remote restart", "peerID", peerID)
	return a.discoveryMgr.SendSystemCommand(peerID, network.ControlCommandRestart)
}

func main() {
	app := &App{}

	err := wails.Run(&options.App{
		Title:      "LanFileTransfer",
		Width:      1024,
		Height:     768,
		MinWidth:   800,
		MinHeight:  600,
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		utils.SugaredLog.Errorw("application error", "error", err)
		os.Exit(1)
	}
}

func init() {
	utils.InitLogger("info")
}
