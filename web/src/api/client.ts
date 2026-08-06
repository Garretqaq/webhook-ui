import axios from 'axios'

const client = axios.create({
  baseURL: '/api',
  withCredentials: true,
})

export interface Hook {
  id: string
  name: string
  command: string
  script_id: string
  ssh_host_id: string
  working_dir: string
  response_message: string
  hmac_secret?: string
  hmac_algorithm: string
  trigger_token?: string
  pass_arguments: string[]
  pass_headers: string[]
  pass_payload_to: string
  created_at: string
  updated_at: string
}

export interface HookListItem extends Hook {
  hmac_enabled: boolean
  trigger_token_enabled: boolean
  script_name: string
  ssh_host_name: string
}

export interface Execution {
  id: number
  hook_id: string
  trigger_source: string
  exec_target: string
  status: string
  output: string
  error: string
  started_at: string
  finished_at?: string
}

export interface ExecutionLogChunk {
  seq: number
  stream: 'stdout' | 'stderr'
  text: string
}

export interface ExecutionLogs {
  chunks: ExecutionLogChunk[]
  next_seq: number
  /** Lowest seq still stored. A cursor below it means chunks were rolled off. */
  oldest_seq: number
  status: string
  finished: boolean
}

export interface Script {
  id: string
  name: string
  interpreter: string
  content: string
  description: string
  created_at: string
  updated_at: string
}

export interface ScriptTestResult {
  success: boolean
  output: string
  error: string
}

export interface SSHHost {
  id: string
  name: string
  host: string
  port: number
  user: string
  auth_type: 'key' | 'password'
  target_os: 'linux' | 'windows'
  credential?: string
  host_key?: string
  created_at: string
  updated_at: string
}

export interface SSHHostTestResult {
  success: boolean
  error?: string
  learned_host_key?: string
}

export const authApi = {
  login: (username: string, password: string) => client.post('/auth/login', { username, password }),
  logout: () => client.post('/auth/logout'),
  check: () => client.get('/auth/check'),
}

export const hookApi = {
  list: () => client.get<HookListItem[]>('/hooks'),
  get: (id: string) => client.get<Hook>(`/hooks/${id}`),
  create: (hook: Partial<Hook>) => client.post<Hook>('/hooks', hook),
  update: (id: string, hook: Partial<Hook>) => client.put<Hook>(`/hooks/${id}`, hook),
  delete: (id: string) => client.delete(`/hooks/${id}`),
}

export const executionApi = {
  list: (params?: { limit?: number; offset?: number; hook_id?: string }) =>
    client.get<Execution[]>('/executions', { params }),
  get: (id: number) => client.get<Execution>(`/executions/${id}`),
  logs: (id: number, afterSeq: number) =>
    client.get<ExecutionLogs>(`/executions/${id}/logs`, { params: { after_seq: afterSeq } }),
}

export const scriptApi = {
  list: () => client.get<Script[]>('/scripts'),
  get: (id: string) => client.get<Script>(`/scripts/${id}`),
  create: (script: Partial<Script>) => client.post<Script>('/scripts', script),
  update: (id: string, script: Partial<Script>) => client.put<Script>(`/scripts/${id}`, script),
  delete: (id: string) => client.delete(`/scripts/${id}`),
  test: (data: { interpreter: string; content: string; args: string[]; ssh_host_id?: string }) =>
    client.post<ScriptTestResult>('/scripts/test', data),
}

export const sshHostApi = {
  list: () => client.get<SSHHost[]>('/ssh-hosts'),
  get: (id: string) => client.get<SSHHost>(`/ssh-hosts/${id}`),
  create: (host: Partial<SSHHost>) => client.post<SSHHost>('/ssh-hosts', host),
  update: (id: string, host: Partial<SSHHost>) => client.put<SSHHost>(`/ssh-hosts/${id}`, host),
  delete: (id: string) => client.delete(`/ssh-hosts/${id}`),
  test: (host: Partial<SSHHost>) => client.post<SSHHostTestResult>('/ssh-hosts/test', host),
}

export default client
