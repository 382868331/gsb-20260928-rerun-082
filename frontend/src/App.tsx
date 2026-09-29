import { useEffect, useState } from "react";
import { useStation } from "./store";
import { TracksPage } from "./components/TracksPage";
import { MarkersPage } from "./components/MarkersPage";

type Page = "tracks" | "markers";

export default function App() {
  const [page, setPage] = useState<Page>("tracks");
  const refresh = useStation((s) => s.refresh);

  useEffect(() => {
    refresh().catch(() => undefined);
  }, [refresh]);

  return (
    <div className="app">
      <header>
        <h1>硬件逻辑分析工作站</h1>
        <nav>
          <button className={page === "tracks" ? "active" : ""} onClick={() => setPage("tracks")}>
            轨道
          </button>
          <button className={page === "markers" ? "active" : ""} onClick={() => setPage("markers")}>
            标记 / 对比
          </button>
        </nav>
      </header>
      <main>{page === "tracks" ? <TracksPage /> : <MarkersPage />}</main>
    </div>
  );
}
