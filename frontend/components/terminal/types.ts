export interface SessionInfo {
  id: string;
  name: string;
}


export type LayoutNode = LayoutPaneNode;

export interface LayoutPaneNode {
  type: "pane";
  id: string;
  sessionId: string;
}

export interface MonitoringActivityPoint {
  timestamp: string;
  score: number;
}

export interface MonitoringSessionSummary {
  id: string;
  name: string;
  command: string;
  process_id: number;
  status: string;
  attention: string;
  last_activity: string;
  cpu_percent: number;
  memory_bytes: number;
  gpu_util: number;
  activity: MonitoringActivityPoint[];
}

export interface MonitoringEvent {
  session_id: string;
  type: string;
  title: string;
  message: string;
  timestamp: string;
}

export interface MonitoringLogbookEntry {
  session_id: string;
  category: string;
  note: string;
  updated_at: string;
}
