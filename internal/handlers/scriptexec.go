package handlers

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/songguangzhi/webhook-ui/internal/database"
	"github.com/songguangzhi/webhook-ui/internal/models"
	"github.com/songguangzhi/webhook-ui/internal/services"
	"golang.org/x/crypto/ssh"
)

// runScript executes a script locally or over SSH depending on sshHostID.
func runScript(executor *services.Executor, interpreter, content, sshHostID string, args []string, env map[string]string, workDir string, out services.OutputStream) *services.ExecuteResult {
	if sshHostID == "" {
		return executor.ExecuteScript(interpreter, content, args, env, workDir, out.Sink)
	}
	return runRemote(sshHostID, func(client *ssh.Client, host *models.SSHHost) *services.ExecuteResult {
		return services.ExecuteScriptSSH(client, host.TargetOS, interpreter, content, args, env, workDir, out)
	})
}

// runCommand executes a hook's free-form command locally (whitelist enforced)
// or over SSH (whitelist does not apply — it describes the local machine).
func runCommand(executor *services.Executor, hook *models.Hook, args []string, env map[string]string, out services.OutputStream) *services.ExecuteResult {
	if hook.SSHHostID == "" {
		return executor.Execute(hook, env, args, out.Sink)
	}
	return runRemote(hook.SSHHostID, func(client *ssh.Client, host *models.SSHHost) *services.ExecuteResult {
		return services.ExecuteCommandSSH(client, host.TargetOS, hook.Command, args, env, hook.WorkingDir, out)
	})
}

// runRemote dials the host, hands the connection to run, and persists any
// host key learned on first use.
func runRemote(sshHostID string, run func(*ssh.Client, *models.SSHHost) *services.ExecuteResult) *services.ExecuteResult {
	host, err := loadSSHHost(sshHostID)
	if err != nil {
		return &services.ExecuteResult{Success: false, Error: fmt.Sprintf("ssh host not found: %s", sshHostID)}
	}

	dialResult, err := services.DialSSH(host)
	if err != nil {
		return &services.ExecuteResult{Success: false, Error: err.Error()}
	}
	defer dialResult.Client.Close()

	persistLearnedHostKey(host.ID, dialResult.LearnedHostKey)

	return run(dialResult.Client, host)
}

// execTarget renders where a hook runs, for the execution log snapshot. It
// is stored as text so history stays readable after a host is deleted.
func execTarget(sshHostID string) string {
	if sshHostID == "" {
		return "local"
	}
	host, err := loadSSHHost(sshHostID)
	if err != nil {
		return "ssh:" + sshHostID
	}
	return fmt.Sprintf("%s@%s:%d", host.User, host.Host, host.Port)
}

// persistLearnedHostKey stores a TOFU-learned host key (no-op when empty).
func persistLearnedHostKey(hostID, learnedKey string) {
	if learnedKey == "" {
		return
	}
	if _, err := database.DB.Exec("UPDATE ssh_hosts SET host_key=? WHERE id=?", learnedKey, hostID); err != nil {
		log.Printf("persist learned host key for %s: %v", hostID, err)
	}
}

func loadSSHHost(id string) (*models.SSHHost, error) {
	var host models.SSHHost
	err := database.DB.QueryRow(`
		SELECT id, name, host, port, user, auth_type, target_os, credential, host_key
		FROM ssh_hosts WHERE id = ?
	`, id).Scan(&host.ID, &host.Name, &host.Host, &host.Port, &host.User,
		&host.AuthType, &host.TargetOS, &host.Credential, &host.HostKey)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("ssh host not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	return &host, nil
}
