import { useEffect, useState } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { Spin } from "antd";
import { me } from "./api";
import type { User } from "./types";
import LoginPage from "./pages/LoginPage";
import ConsoleLayout from "./pages/ConsoleLayout";
import DashboardPage from "./pages/DashboardPage";
import DefinitionsPage from "./pages/DefinitionsPage";
import DefinitionDetailPage from "./pages/DefinitionDetailPage";
import SchedulesPage from "./pages/SchedulesPage";
import RunsPage from "./pages/RunsPage";
import RunDetailPage from "./pages/RunDetailPage";
import StatsPage from "./pages/StatsPage";
import TargetsPage from "./pages/TargetsPage";
import CredentialsPage from "./pages/CredentialsPage";
import UsersPage from "./pages/UsersPage";
import ProfilePage from "./pages/ProfilePage";
import AuditPage from "./pages/AuditPage";

export default function App() {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    me()
      .then(setUser)
      .catch(() => setUser(null))
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <Spin fullscreen />;
  if (!user) {
    return (
      <Routes>
        <Route path="/login" element={<LoginPage onLogin={setUser} />} />
        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    );
  }

  return (
    <Routes>
      <Route element={<ConsoleLayout user={user} onLogout={() => setUser(null)} />}>
        <Route path="/" element={<DashboardPage />} />
        <Route path="/definitions" element={<DefinitionsPage user={user} />} />
        <Route path="/definitions/:id" element={<DefinitionDetailPage user={user} />} />
        <Route path="/schedules" element={<SchedulesPage user={user} />} />
        <Route path="/runs" element={<RunsPage user={user} />} />
        <Route path="/runs/:id" element={<RunDetailPage user={user} />} />
        <Route path="/stats" element={<StatsPage />} />
        {user.role === "admin" && <Route path="/targets" element={<TargetsPage />} />}
        {user.role === "admin" && <Route path="/credentials" element={<CredentialsPage />} />}
        {user.role === "admin" && <Route path="/users" element={<UsersPage />} />}
        {user.role === "admin" && <Route path="/audit" element={<AuditPage />} />}
        <Route path="/profile" element={<ProfilePage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  );
}
