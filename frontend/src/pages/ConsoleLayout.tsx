import { Layout, Menu, Space, Tag, Typography, Button } from "antd";
import type { MenuProps } from "antd";
import { DashboardOutlined, ScheduleOutlined, UnorderedListOutlined, AppstoreOutlined, LogoutOutlined, BarChartOutlined, CloudServerOutlined, KeyOutlined, TeamOutlined, UserOutlined, AuditOutlined } from "@ant-design/icons";
import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";
import { logout } from "../api";
import type { User } from "../types";

const { Header, Sider, Content } = Layout;

export default function ConsoleLayout({ user, onLogout }: { user: User; onLogout: () => void }) {
  const location = useLocation();
  const navigate = useNavigate();
  const selected = location.pathname.startsWith("/definitions")
    ? "/definitions"
    : location.pathname.startsWith("/schedules")
      ? "/schedules"
      : location.pathname.startsWith("/runs")
        ? "/runs"
        : location.pathname.startsWith("/stats")
          ? "/stats"
          : location.pathname.startsWith("/targets")
            ? "/targets"
            : location.pathname.startsWith("/credentials")
              ? "/credentials"
              : location.pathname.startsWith("/users")
                ? "/users"
                : location.pathname.startsWith("/audit")
                  ? "/audit"
                  : "/";

  const monitoringItems: MenuProps["items"] = [
    { key: "/", icon: <DashboardOutlined />, label: <Link to="/">仪表</Link> },
    { key: "/stats", icon: <BarChartOutlined />, label: <Link to="/stats">统计</Link> },
    { key: "/runs", icon: <UnorderedListOutlined />, label: <Link to="/runs">运行</Link> },
  ];

  const taskItems: MenuProps["items"] = [
    { key: "/definitions", icon: <AppstoreOutlined />, label: <Link to="/definitions">任务</Link> },
    { key: "/schedules", icon: <ScheduleOutlined />, label: <Link to="/schedules">调度</Link> },
    ...(user.role === "admin"
      ? [
          { key: "/credentials", icon: <KeyOutlined />, label: <Link to="/credentials">凭据</Link> },
          { key: "/targets", icon: <CloudServerOutlined />, label: <Link to="/targets">执行目标</Link> },
        ]
      : []),
  ];

  const menuItems: MenuProps["items"] = [
    { type: "group", label: "监控", children: monitoringItems },
    { type: "group", label: "任务配置", children: taskItems },
    ...(user.role === "admin"
      ? [
          {
            type: "group" as const,
            label: "系统管理",
            children: [
              { key: "/users", icon: <TeamOutlined />, label: <Link to="/users">用户</Link> },
              { key: "/audit", icon: <AuditOutlined />, label: <Link to="/audit">审计</Link> },
            ],
          },
        ]
      : []),
  ];

  async function signOut() {
    await logout().catch(() => undefined);
    onLogout();
    navigate("/login");
  }

  return (
    <Layout className="app-shell">
      <Sider breakpoint="lg" collapsedWidth="0" theme="dark">
        <div className="brand">任务中控台</div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[selected]}
          items={menuItems}
        />
      </Sider>
      <Layout>
        <Header className="topbar">
          <Typography.Text>{user.display_name}</Typography.Text>
          <Space>
            <Tag color="blue">{user.role}</Tag>
            <Button icon={<UserOutlined />} onClick={() => navigate("/profile")}>个人设置</Button>
            <Button icon={<LogoutOutlined />} onClick={signOut}>
              退出
            </Button>
          </Space>
        </Header>
        <Content className="content">
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  );
}
