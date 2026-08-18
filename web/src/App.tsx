import { useEffect, useRef, useState } from 'preact/hooks';
import './App.css';

type Status = { apache: boolean; mariadb: boolean };
type Proc = { pid: number; cpu: number; ram: number };
type Procs = { apache: Proc[]; mariadb: Proc[] };

type LogEntry = { service: 'apache' | 'mariadb' | 'system'; line: string };

const tabs = ['Services', 'Databases', 'Config', 'About'] as const;
type Tab = (typeof tabs)[number];

const dbs = ['app_werd', 'blog'];
const ports = ['8080 (httpd)', '3306 (mariadb)'];

const wsUrl = (() => {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  return `${proto}://${location.host}/ws`;
})();

export const App = () => {
  const [tab, setTab] = useState<Tab>('Services');
  const [status, setStatus] = useState<Status>({ apache: false, mariadb: false });
  const [procs, setProcs] = useState<Procs>({ apache: [], mariadb: [] });
  const [loading, setLoading] = useState<{ apache: boolean; mariadb: boolean }>({
    apache: false,
    mariadb: false,
  });
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    let closed = false;
    const connect = () => {
      const ws = new WebSocket(wsUrl);
      wsRef.current = ws;
      ws.onopen = () => {
        if (closed) return;
        setConnected(true);
        setLogs((l) => [...l, { service: 'system', line: 'connected' }]);
      };
      ws.onclose = () => {
        if (closed) return;
        setConnected(false);
        setLoading({ apache: false, mariadb: false });
        setTimeout(connect, 3000);
      };
      ws.onmessage = (ev) => {
        let msg: any;
        try {
          msg = JSON.parse(ev.data);
        } catch {
          return;
        }
        if (msg.type === 'status') {
          setStatus((s) => ({ ...s, [msg.service]: msg.running }));
          setProcs((p) => ({ ...p, [msg.service]: msg.procs || [] }));
          setLoading((l) => ({ ...l, [msg.service]: false }));
        } else if (msg.type === 'log') {
          setLogs((l) => [...l.slice(-199), { service: msg.service, line: msg.line }]);
        } else if (msg.type === 'error') {
          setLogs((l) => [...l.slice(-199), { service: 'system', line: `[error] ${msg.message}` }]);
        }
      };
    };
    connect();
    return () => {
      closed = true;
      wsRef.current?.close();
    };
  }, []);

  const control = (service: 'apache' | 'mariadb', action: 'start' | 'stop') => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    setLoading((l) => ({ ...l, [service]: true }));
    ws.send(JSON.stringify({ action, service }));
  };

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-100 antialiased">
      <div className="mx-auto flex w-full max-w-7xl flex-col gap-6 px-6 py-8">
        <Header running={connected && status.apache && status.mariadb} connected={connected} />
        <Tabs current={tab} onSelect={setTab} />
        {tab === 'Services' && (
          <Services
            status={status}
            procs={procs}
            loading={loading}
            onControl={control}
            onAll={(a) => {
              const ws = wsRef.current;
              if (!ws || ws.readyState !== WebSocket.OPEN) return;
              setLoading({ apache: true, mariadb: true });
              ws.send(JSON.stringify({ action: a }));
            }}
          />
        )}
        {tab === 'Databases' && <Databases />}
        {tab === 'Config' && <Config />}
        {tab === 'About' && <About />}
        <Logs logs={logs} />
      </div>
    </div>
  );
};

const Header = ({ running, connected }: { running: boolean; connected: boolean }) => (
  <header className="flex items-center justify-between">
    <div>
      <h1 className="text-2xl font-semibold tracking-tight">WERD Panel</h1>
      <p className="text-sm text-zinc-400">localhost · control panel</p>
    </div>
    <span
      className={`inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium ${
        running
          ? 'bg-emerald-500/15 text-emerald-400'
          : connected
            ? 'bg-amber-500/15 text-amber-400'
            : 'bg-red-500/15 text-red-400'
      }`}
    >
      <span
        className={`size-2 rounded-full ${
          running ? 'bg-emerald-400' : connected ? 'bg-amber-400' : 'bg-red-400'
        }`}
      />
      {!connected ? 'Offline' : running ? 'Running' : 'Stopped'}
    </span>
  </header>
);

const Tabs = ({ current, onSelect }: { current: Tab; onSelect: (t: Tab) => void }) => (
  <nav className="flex gap-1 rounded-lg border border-zinc-800 bg-zinc-900 p-1">
    {tabs.map((t) => (
      <button
        key={t}
        onClick={() => onSelect(t)}
        className={`rounded-lg px-4 py-2 text-sm font-medium transition-colors ${
          current === t
            ? 'bg-zinc-800 text-zinc-50'
            : 'text-zinc-400 hover:bg-zinc-800/60 hover:text-zinc-200'
        }`}
      >
        {t}
      </button>
    ))}
  </nav>
);

const ServiceCard = ({
  name,
  version,
  port,
  dir,
  running,
  procs,
  loading,
  onControl,
}: {
  name: string;
  version: string;
  port: string;
  dir: string;
  running: boolean;
  procs: Proc[];
  loading: boolean;
  onControl: (a: 'start' | 'stop') => void;
}) => (
  <div className="flex items-center justify-between rounded-lg border border-zinc-800 bg-zinc-900 p-4">
    <div className="flex items-center gap-3">
      <span className={`size-2.5 rounded-full ${running ? 'bg-emerald-500' : 'bg-red-500'}`} />
      <div>
        <div className="text-sm font-medium text-zinc-100">{name}</div>
        <div className="text-xs text-zinc-400">
          {version} · port {port}
        </div>
        {running && procs.length > 0 && (
          <div className="mt-1 text-[11px] text-zinc-500">
            {procs.length > 1 ? `${procs.length} processes` : `pid ${procs[0].pid}`}
            {' · '}cpu {Math.round(procs.reduce((s, p) => s + p.cpu, 0) * 10) / 10}%
            {' · '}ram {Math.round(procs.reduce((s, p) => s + p.ram, 0) * 10) / 10} MB
          </div>
        )}
      </div>
    </div>
    <div className="flex items-center gap-2">
      <span className="text-xs text-zinc-500">{dir}</span>
      <button
        disabled={loading || running}
        onClick={() => onControl('start')}
        title={running ? 'Already running' : loading ? 'Busy' : undefined}
        className="rounded-lg bg-emerald-600 px-3 py-1.5 text-xs font-medium text-emerald-950 hover:bg-emerald-500 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {loading ? <Spinner /> : 'Start'}
      </button>
      <button
        disabled={loading || !running}
        onClick={() => onControl('stop')}
        title={!running ? 'Not running' : loading ? 'Busy' : undefined}
        className="rounded-lg border border-zinc-700 px-3 py-1.5 text-xs font-medium text-zinc-300 hover:bg-zinc-800 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {loading ? <Spinner /> : 'Stop'}
      </button>
    </div>
  </div>
);

const Services = ({
  status,
  procs,
  loading,
  onControl,
  onAll,
}: {
  status: Status;
  procs: Procs;
  loading: { apache: boolean; mariadb: boolean };
  onControl: (s: 'apache' | 'mariadb', a: 'start' | 'stop') => void;
  onAll: (a: 'startAll' | 'stopAll') => void;
}) => (
  <div className="grid gap-4">
    <div className="flex items-center gap-2">
      <button
        disabled={loading.apache || loading.mariadb || (status.apache && status.mariadb)}
        onClick={() => onAll('startAll')}
        title={
          status.apache && status.mariadb
            ? 'All services already running'
            : loading.apache || loading.mariadb
              ? 'Busy'
              : undefined
        }
        className="rounded-lg bg-emerald-600 px-4 py-2 text-xs font-medium text-emerald-950 hover:bg-emerald-500 disabled:cursor-not-allowed disabled:opacity-50"
      >
        Start All
      </button>
      <button
        disabled={loading.apache || loading.mariadb || (!status.apache && !status.mariadb)}
        onClick={() => onAll('stopAll')}
        title={
          !status.apache && !status.mariadb
            ? 'All services already stopped'
            : loading.apache || loading.mariadb
              ? 'Busy'
              : undefined
        }
        className="rounded-lg border border-zinc-700 px-4 py-2 text-xs font-medium text-zinc-300 hover:bg-zinc-800 disabled:cursor-not-allowed disabled:opacity-50"
      >
        Stop All
      </button>
    </div>
    <ServiceCard
      name="Apache"
      version="2.4.66"
      port="8080"
      dir="bin/httpd-2.4.66"
      running={status.apache}
      procs={procs.apache}
      loading={loading.apache}
      onControl={(a) => onControl('apache', a)}
    />
    <ServiceCard
      name="MariaDB"
      version="12.3.2"
      port="3306"
      dir="bin/mariadb-12.3.2"
      running={status.mariadb}
      procs={procs.mariadb}
      loading={loading.mariadb}
      onControl={(a) => onControl('mariadb', a)}
    />
    <div className="flex items-center justify-between rounded-lg border border-zinc-800 bg-zinc-900 p-4 opacity-60">
      <div className="flex items-center gap-3">
        <span className="size-2.5 rounded-full bg-emerald-500" />
        <div>
          <div className="text-sm font-medium text-zinc-100">PHP</div>
          <div className="text-xs text-zinc-400">8.4.23 · mod_php</div>
        </div>
      </div>
      <span className="text-xs text-zinc-500">via Apache</span>
    </div>
    <div className="flex items-center justify-between rounded-lg border border-zinc-800 bg-zinc-900 p-4 opacity-60">
      <div className="flex items-center gap-3">
        <span className="size-2.5 rounded-full bg-emerald-500" />
        <div>
          <div className="text-sm font-medium text-zinc-100">phpMyAdmin</div>
          <div className="text-xs text-zinc-400">5.2.3 · :8080</div>
        </div>
      </div>
      <a
        href="http://127.0.0.1:8080"
        target="_blank"
        rel="noreferrer"
        className="rounded-lg border border-zinc-700 px-3 py-1.5 text-xs font-medium text-zinc-300 hover:bg-zinc-800"
      >
        Open
      </a>
    </div>
  </div>
);

const Spinner = () => (
  <span className="inline-block size-3 animate-spin rounded-full border-2 border-current border-t-transparent align-[-1px]" />
);

const Logs = ({ logs }: { logs: LogEntry[] }) => {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (ref.current) ref.current.scrollTop = ref.current.scrollHeight;
  }, [logs]);
  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-950">
      <div className="flex items-center gap-2 border-b border-zinc-800 px-4 py-2 text-xs text-zinc-400">
        <span className="size-1.5 rounded-full bg-zinc-500" />
        Logs
      </div>
      <div
        ref={ref}
        className="h-64 overflow-y-auto px-4 py-3 font-mono text-xs leading-relaxed text-zinc-400"
      >
        {logs.length === 0 ? (
          <div className="text-zinc-600">no output yet</div>
        ) : (
          logs.map((l, i) => (
            <div key={i} className="whitespace-pre-wrap">
              <span
                className={
                  l.service === 'apache'
                    ? 'text-sky-400'
                    : l.service === 'mariadb'
                      ? 'text-emerald-400'
                      : 'text-zinc-500'
                }
              >
                [{l.service}]
              </span>{' '}
              {l.line}
            </div>
          ))
        )}
      </div>
    </div>
  );
};

const Databases = () => (
  <div className="rounded-lg border border-zinc-800 bg-zinc-900 p-4">
    <div className="grid gap-2">
      {dbs.map((db) => (
        <div key={db} className="flex items-center justify-between rounded-lg px-2 py-2 hover:bg-zinc-800/60">
          <span className="text-sm text-zinc-100">{db}</span>
          <button className="rounded-lg border border-zinc-700 px-3 py-1 text-xs text-zinc-300 hover:bg-zinc-800">
            Open
          </button>
        </div>
      ))}
    </div>
  </div>
);

const Config = () => (
  <div className="rounded-lg border border-zinc-800 bg-zinc-900 p-4">
    <h2 className="mb-3 text-sm font-semibold text-zinc-100">Configuration</h2>
    <div className="grid gap-2">
      {ports.map((p) => (
        <div key={p} className="rounded-lg px-2 py-1 text-sm text-zinc-300">
          {p}
        </div>
      ))}
    </div>
  </div>
);

const About = () => (
  <div className="rounded-lg border border-zinc-800 bg-zinc-900 p-4">
    <h2 className="mb-1 text-sm font-semibold text-zinc-100">WERD Panel</h2>
    <p className="text-sm text-zinc-400">Control panel — alternative to XAMPP / Laragon.</p>
    <p className="mt-2 text-xs text-zinc-500">Apache · MariaDB · PHP · phpMyAdmin</p>
  </div>
);

export default App;