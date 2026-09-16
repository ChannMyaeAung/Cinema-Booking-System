import { HashRouter, Route, Routes } from "react-router-dom";
import Home from "./pages/Home";
import SeatMap from "./pages/SeatMap";
import { AuroraBackground } from "@/components/ui/aurora-background";
import Navbar from "@/components/Navbar";

function App() {
  return (
    <HashRouter>
      <div className="relative min-h-svh overflow-x-clip">
        <AuroraBackground />

        <div className="relative z-10 flex min-h-svh flex-col">
          <Navbar />

          <main className="flex-1 px-5 pb-24 pt-8">
            <div className="mx-auto max-w-6xl">
              <Routes>
                <Route path="/" element={<Home />} />
                <Route path="/movie/:movieID" element={<SeatMap />} />
              </Routes>
            </div>
          </main>
        </div>
      </div>
    </HashRouter>
  );
}

export default App;
