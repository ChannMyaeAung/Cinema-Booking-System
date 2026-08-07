import { HashRouter, Route, Routes } from "react-router-dom";
import Home from "./pages/Home";
import SeatMap from "./pages/SeatMap";
import { Show, SignInButton, SignUpButton, UserButton } from "@clerk/react";

function App() {
  return (
    <HashRouter>
      <header className="app-header">
        <div className="app-header-inner">
          <a href="#/" className="brand">
            <span className="brand-mark" aria-hidden="true">
              ◆
            </span>
            CineBook
          </a>
          <div className="header-user">
            <Show when="signed-out">
              <SignInButton />
              <SignUpButton />
            </Show>
            <Show when="signed-in">
              <UserButton />
            </Show>
          </div>
        </div>
      </header>

      <main className="app-main">
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/movie/:movieID" element={<SeatMap />} />
        </Routes>
      </main>
    </HashRouter>
  );
}

export default App;
