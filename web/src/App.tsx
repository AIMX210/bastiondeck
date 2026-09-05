import { HashRouter, Navigate, Route, Routes } from 'react-router-dom'
import { AuthProvider } from './store/auth'
import { ToastProvider } from './lib/toast'
import { Gate, RequirePerm } from './components/Guard'
import { Layout } from './components/Layout'
import { LoginPage } from './pages/Login'
import { SetupPage } from './pages/Setup'
import { DashboardPage } from './pages/Dashboard'
import { HostsPage } from './pages/Hosts'
import { HostDetailPage } from './pages/HostDetail'
import { FilesPage } from './pages/Files'
import { ExecWizardPage } from './pages/ExecWizard'
import { RunsPage } from './pages/Runs'
import { RunDetailPage } from './pages/RunDetail'
import { JobsPage } from './pages/Jobs'
import { TunnelsPage } from './pages/Tunnels'
import { SnippetsPage } from './pages/Snippets'
import { CredentialsPage } from './pages/Credentials'
import { AgentsPage } from './pages/Agents'
import { AuditPage } from './pages/Audit'
import { SettingsPage } from './pages/Settings'
import { UsersPage } from './pages/Users'
import { BackupPage } from './pages/Backup'

function NotFound() {
  return (
    <div className="topbar"><h1>页面不存在</h1></div>
  )
}

export default function App() {
  return (
    <ToastProvider>
      <AuthProvider>
        <HashRouter>
          <Routes>
            <Route path="/login" element={<LoginPage />} />
            <Route path="/setup" element={<SetupPage />} />
            <Route element={<Gate><Layout /></Gate>}>
              <Route path="/" element={<DashboardPage />} />
              <Route path="/hosts" element={<HostsPage />} />
              <Route path="/hosts/:id" element={<HostDetailPage />} />
              <Route path="/files" element={<RequirePerm perm="exec"><FilesPage /></RequirePerm>} />
              <Route path="/exec" element={<RequirePerm perm="exec"><ExecWizardPage /></RequirePerm>} />
              <Route path="/runs" element={<RunsPage />} />
              <Route path="/runs/:id" element={<RunDetailPage />} />
              <Route path="/jobs" element={<JobsPage />} />
              <Route path="/tunnels" element={<TunnelsPage />} />
              <Route path="/snippets" element={<SnippetsPage />} />
              <Route path="/credentials" element={<RequirePerm perm="exec"><CredentialsPage /></RequirePerm>} />
              <Route path="/agents" element={<RequirePerm perm="manage_inventory"><AgentsPage /></RequirePerm>} />
              <Route path="/audit" element={<RequirePerm perm="audit"><AuditPage /></RequirePerm>} />
              <Route path="/settings" element={<SettingsPage />} />
              <Route path="/users" element={<RequirePerm perm="manage_users"><UsersPage /></RequirePerm>} />
              <Route path="/backup" element={<RequirePerm perm="owner"><BackupPage /></RequirePerm>} />
              <Route path="*" element={<NotFound />} />
            </Route>
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </HashRouter>
      </AuthProvider>
    </ToastProvider>
  )
}
