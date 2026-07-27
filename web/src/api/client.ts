import axios from 'axios'

const client = axios.create({
  baseURL: '/api',
  withCredentials: true,
})

export interface Hook {
  id: string
  name: string
  command: string
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
}

export interface Execution {
  id: number
  hook_id: string
  trigger_source: string
  status: string
  output: string
  error: string
  started_at: string
  finished_at?: string
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

export const authApi = {
  login: (password: string) => client.post('/auth/login', { password }),
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
}

export const scriptApi = {
  list: () => client.get<Script[]>('/scripts'),
  get: (id: string) => client.get<Script>(`/scripts/${id}`),
  create: (script: Partial<Script>) => client.post<Script>('/scripts', script),
  update: (id: string, script: Partial<Script>) => client.put<Script>(`/scripts/${id}`, script),
  delete: (id: string) => client.delete(`/scripts/${id}`),
  test: (data: { interpreter: string; content: string; args: string[] }) =>
    client.post<ScriptTestResult>('/scripts/test', data),
}

export default client
