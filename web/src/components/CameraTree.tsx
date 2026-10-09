import { useMemo, useState } from "react";
import type { Camera } from "../api";

// CameraTree lists cameras grouped by their group name, like the device tree of a VMS client.
export function CameraTree({
  cameras,
  selected,
  onPick,
  draggable,
}: {
  cameras: Camera[];
  selected?: number | null;
  onPick: (c: Camera) => void;
  draggable?: boolean;
}) {
  const [filter, setFilter] = useState("");
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});

  const groups = useMemo(() => {
    const f = filter.trim().toLowerCase();
    const map = new Map<string, Camera[]>();
    for (const c of cameras) {
      if (f && !c.name.toLowerCase().includes(f) && !c.host.toLowerCase().includes(f)) continue;
      const g = c.group || "Без группы";
      if (!map.has(g)) map.set(g, []);
      map.get(g)!.push(c);
    }
    return [...map.entries()];
  }, [cameras, filter]);

  return (
    <div className="tree">
      <input className="tree-search" placeholder="Поиск камеры…" value={filter} onChange={(e) => setFilter(e.target.value)} />
      <div className="tree-body">
        {groups.length === 0 && <div className="muted pad">Камер нет</div>}
        {groups.map(([group, cams]) => (
          <div key={group}>
            <button className="tree-group" onClick={() => setCollapsed((s) => ({ ...s, [group]: !s[group] }))}>
              <span>{collapsed[group] ? "▸" : "▾"}</span> {group} <span className="muted">({cams.length})</span>
            </button>
            {!collapsed[group] &&
              cams.map((c) => (
                <button
                  key={c.id}
                  className={`tree-item ${selected === c.id ? "active" : ""}`}
                  onClick={() => onPick(c)}
                  draggable={draggable}
                  onDragStart={(e) => e.dataTransfer.setData("text/camera", String(c.id))}
                  title={c.enabled ? (c.status?.online ? "В сети" : "Нет сигнала") : "Отключена"}
                >
                  <span className={`dot ${!c.enabled ? "off" : c.status?.online ? "ok" : "bad"}`} />
                  <span className="tree-name">{c.name}</span>
                  {c.record && c.enabled && <span className="rec" title="Идёт запись">●</span>}
                </button>
              ))}
          </div>
        ))}
      </div>
    </div>
  );
}
