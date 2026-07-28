import { Link, useLocation } from "react-router-dom";
import {
  Activity,
  Bookmark,
  BookOpen,
  FileText,
  LayoutDashboard,
  ListChecks,
  Monitor,
  Settings,
  Database,
  ChevronLeft,
  ChevronRight,
  Layers,
  PieChart,
  Cpu,
  Globe,
  Bot,
  Puzzle,
  Tag,
  type LucideIcon,
} from "lucide-react";
import clsx from "clsx";
import { useLayoutStore } from "../../store/layoutStore";
import { projectConfig, navigationConfig } from "../../config";
import { Logo } from "../components";

const iconMap: Record<string, LucideIcon> = {
  LayoutDashboard,
  Settings,
  Database,
  Layers,
  PieChart,
  Monitor,
  ListChecks,
  Activity,
  FileText,
  Cpu,
  Globe,
  Bot,
  Puzzle,
  Bookmark,
  BookOpen,
  Tag,
};

function getIcon(iconName: string): LucideIcon {
  return iconMap[iconName] || LayoutDashboard;
}

export function Sidebar() {
  const location = useLocation();
  const { sidebarCollapsed, toggleSidebar } = useLayoutStore();

  return (
    <aside
      className={clsx(
        "bg-[var(--color-bg-surface)] flex flex-col transition-all duration-300 border-r border-[var(--border-subtle)]",
        sidebarCollapsed ? "w-16" : "w-60",
      )}
    >
      {/* Logo */}
      <div
        className={clsx(
          "h-14 flex items-center border-b border-[var(--border-subtle)]",
          sidebarCollapsed ? "justify-center px-2" : "px-5",
        )}
      >
        {!sidebarCollapsed ? (
          <div className="flex items-center gap-2.5">
            <Logo size={24} className="flex-shrink-0" />
            <h2 className="text-[17px] font-semibold text-[var(--text-primary)] tracking-tight truncate">
              {projectConfig.name}
            </h2>
          </div>
        ) : (
          <Logo size={28} className="flex-shrink-0" />
        )}
      </div>

      {/* Navigation */}
      <nav className="flex-1 py-4 px-3 space-y-6 overflow-y-auto">
        {navigationConfig.map((section) => (
          <div key={section.title}>
            {!sidebarCollapsed && (
              <h3 className="px-3 mb-2 mt-3 first:mt-0 text-[11px] font-semibold text-[var(--text-muted)] uppercase tracking-[0.06em]">
                {section.title}
              </h3>
            )}
            <div className="space-y-1">
              {section.items.map((item) => {
                const Icon = getIcon(item.icon);
                const isActive =
                  location.pathname === item.path ||
                  (item.path !== "/" &&
                    location.pathname.startsWith(`${item.path}/`));

                return (
                  <Link
                    key={item.path}
                    to={item.path}
                    title={sidebarCollapsed ? item.name : undefined}
                    className={clsx(
                      "relative flex items-center rounded-[var(--radius-md)] transition-all duration-150",
                      isActive
                        ? "bg-[var(--accent-soft)] text-[var(--accent)]"
                        : "text-[var(--text-secondary)] hover:bg-[var(--bg-hover)] hover:text-[var(--text-primary)]",
                      sidebarCollapsed
                        ? "justify-center w-10 h-10 mx-auto"
                        : "h-[34px] px-3 gap-2",
                    )}
                  >
                    {isActive && !sidebarCollapsed && (
                      <span
                        aria-hidden="true"
                        className="absolute left-0 top-1/2 -translate-y-1/2 h-[18px] w-0.5 rounded-full bg-[var(--accent)]"
                      />
                    )}
                    <Icon
                      className="w-4 h-4 flex-shrink-0"
                      strokeWidth={1.75}
                    />
                    {!sidebarCollapsed && (
                      <span className="text-[13px] truncate">
                        {item.name}
                      </span>
                    )}
                  </Link>
                );
              })}
            </div>
          </div>
        ))}
      </nav>

      {/* Toggle Button */}
      <div className="p-3 border-t border-[var(--border-subtle)]">
        <button
          onClick={toggleSidebar}
          className={clsx(
            "flex items-center rounded-[var(--radius-md)] text-[var(--text-muted)] hover:bg-[var(--bg-hover)] hover:text-[var(--text-primary)] transition-all duration-150",
            sidebarCollapsed
              ? "justify-center w-10 h-10 mx-auto"
              : "w-full h-[34px] px-3 gap-2",
          )}
          title={sidebarCollapsed ? "展开" : "收起"}
        >
          {sidebarCollapsed ? (
            <ChevronRight className="w-4 h-4" strokeWidth={1.75} />
          ) : (
            <>
              <ChevronLeft className="w-4 h-4" strokeWidth={1.75} />
              <span className="text-[13px]">收起侧边栏</span>
            </>
          )}
        </button>
      </div>
    </aside>
  );
}
