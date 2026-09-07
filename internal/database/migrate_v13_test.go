package database

import "testing"

func TestMigrateV13PreservesExistingHooksAndHistory(t *testing.T) {
	openTestDB(t)
	if err := migrateTo(12); err != nil {
		t.Fatal(err)
	}
	mustExec(t, `INSERT INTO hooks (id,name,command,async,timeout_seconds) VALUES ('old','old','echo old',1,0)`)
	mustExec(t, `INSERT INTO executions (hook_id,status,output,finished_at) VALUES ('old','success','old output',CURRENT_TIMESTAMP)`)
	if err := Migrate(); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(); err != nil {
		t.Fatal(err)
	}
	var target, body, command string
	var timeout int
	if err := DB.QueryRow("SELECT success_hook_id,success_body,command,timeout_seconds FROM hooks WHERE id='old'").Scan(&target, &body, &command, &timeout); err != nil {
		t.Fatal(err)
	}
	if target != "" || body != "" || command != "echo old" || timeout != 0 {
		t.Fatalf("migration changed old hook: %q %q %q %d", target, body, command, timeout)
	}
	var status, output, handoff string
	var parent int64
	if err := DB.QueryRow("SELECT status,output,success_status,parent_execution_id FROM executions WHERE hook_id='old'").Scan(&status, &output, &handoff, &parent); err != nil {
		t.Fatal(err)
	}
	if status != "success" || output != "old output" || handoff != "" || parent != 0 {
		t.Fatalf("migration changed history: %q %q %q %d", status, output, handoff, parent)
	}
}
