import { HashRouter, Route, Routes } from 'react-router-dom'
import Home from './pages/Home'
import SeatMap from './pages/SeatMap'

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
        </div>
      </header>

      <main className="app-main">
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/movie/:movieID" element={<SeatMap />} />
        </Routes>
      </main>
    </HashRouter>
  )
}

export default App
