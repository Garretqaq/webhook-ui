package handlers

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/songguangzhi/webhook-ui/internal/database"
	"github.com/songguangzhi/webhook-ui/internal/models"
	"github.com/songguangzhi/webhook-ui/internal/services"
)

// runScript executes a script locally or over SSH depending on sshHostID.
// workDir only applies to local execution.
func runScript(executor *services.Executor, interpreter, content, sshHostID string, args []string, env map[string]string, workDir string) *services.ExecuteResult {
	if sshHostID == "" {
		return executor.ExecuteScript(interpreter, content, args, env, workDir)
	}

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

	return services.ExecuteScriptSSH(dialResult.Client, interpreter, content, args, env)
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
		SELECT id, name, host, port, user, auth_type, credential, host_key
		FROM ssh_hosts WHERE id = ?
	`, id).Scan(&host.ID, &host.Name, &host.Host, &host.Port, &host.User,
		&host.AuthType, &host.Credential, &host.HostKey)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("ssh host not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	return &host, nil
}
