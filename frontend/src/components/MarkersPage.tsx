import { useEffect, useMemo, useState } from "react";
import { api } from "../api";
import { useStation } from "../store";
import type { DecodeResult } from "../types";
import { formatNs } from "../time";

export function MarkersPage() {
  const { result, diff, runCompare, busy } = useStation();
  const [versions, setVersions] = useState<string[]>([]);
  const [a, setA] = useState("");
  const [b, setB] = useState("");
  const [fetched, setFetched] = useState<Record<string, DecodeResult>>({});

  useEffect(() => {
    api.results().then((vs) => {
      setVersions(vs);
      if (!a && vs.length) setA(vs[vs.length - 1]);
      if (!b && vs.length > 1) setB(vs[vs.length - 2]);
    });
  }, [result, a, b]);

  useEffect(() => {
    for (const v of [a, b]) {
      if (v && !fetched[v]) {
        api.result(v).then((r) => setFetched((f) => ({ ...f, [v]: r })));
      }
    }
  }, [a, b, fetched]);

  const items = useMemo(() => result?.items ?? [], [result]);
  const errors = items.filter((i) => i.kind === "error");
  const data = items.filter((i) => i.kind === "data");

  return (
    <div>
      <section className="panel">
        <h2>标记</h2>
        {!result && <p className="hint">还没有结果：到“轨道”页生成波形并重算。</p>}
        <div className="marker-grid">
          <div>
            <h3>数据帧（{data.length}）</h3>
            <table>
              <thead>
                <tr>
                  <th>起始</th>
                  <th>结束</th>
                  <th>字节</th>
                  <th>位</th>
                </tr>
              </thead>
              <tbody>
                {data.map((it, i) => (
                  <tr key={i}>
                    <td>{formatNs(it.startNs)}</td>
                    <td>{formatNs(it.endNs)}</td>
                    <td>
                      <code>
                        0x{it.value!.toString(16).padStart(2, "0")}
                      </code>{" "}
                      {it.value! >= 32 && it.value! < 127 ? String.fromCharCode(it.value!) : ""}
                    </td>
                    <td className="bits">
                      {(it.bits ?? []).map((bt) => bt.level).join("")}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div>
            <h3 className="er">错误（{errors.length}，保留不修补）</h3>
            <table>
              <thead>
                <tr>
                  <th>起始</th>
                  <th>类型</th>
                  <th>说明</th>
                </tr>
              </thead>
              <tbody>
                {errors.map((it, i) => (
                  <tr key={i}>
                    <td>{formatNs(it.startNs)}</td>
                    <td className="er">{it.errCode}</td>
                    <td>{it.message}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </section>

      <section className="panel">
        <h2>结果版本对比（改配置重算后可比较）</h2>
        <div className="row">
          <select value={a} onChange={(e) => setA(e.target.value)}>
            {versions.map((v) => (
              <option key={v} value={v}>
                {v.slice(0, 18)}…
              </option>
            ))}
          </select>
          <span>vs</span>
          <select value={b} onChange={(e) => setB(e.target.value)}>
            {versions.map((v) => (
              <option key={v} value={v}>
                {v.slice(0, 18)}…
              </option>
            ))}
          </select>
          <button disabled={busy || !a || !b} onClick={() => runCompare(a, b)}>
            比较
          </button>
        </div>
        {diff && (
          <div>
            <p className={diff.same ? "ok" : "er"}>
              {diff.same ? "两个结果完全一致（按起始时刻对齐）" : "结果存在差异"}
            </p>
            <div className="marker-grid">
              <div>
                <h3>仅 A 有（{diff.onlyInA.length}）</h3>
                <DiffList items={diff.onlyInA} />
              </div>
              <div>
                <h3>仅 B 有（{diff.onlyInB.length}）</h3>
                <DiffList items={diff.onlyInB} />
              </div>
            </div>
            <h3>变化（{diff.changed.length}）</h3>
            <table>
              <tbody>
                {diff.changed.slice(0, 50).map((c, i) => (
                  <tr key={i}>
                    <td>{formatNs(c.startNs)}</td>
                    <td className="er">{desc(c.a)}</td>
                    <td>→</td>
                    <td>{desc(c.b)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  );
}

function desc(it: { kind: string; value?: number; errCode?: string }): string {
  if (it.kind === "data" && it.value !== undefined) return `0x${it.value.toString(16).padStart(2, "0")}`;
  return `ERR:${it.errCode}`;
}

function DiffList({ items }: { items: DecodeResult["items"] }) {
  if (!items.length) return <p className="hint">无</p>;
  return (
    <table>
      <tbody>
        {items.map((it, i) => (
          <tr key={i}>
            <td>{formatNs(it.startNs)}</td>
            <td>{desc(it)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
