import { Link, useParams } from 'react-router-dom'

export default function SeatMap() {
  const { movieID } = useParams<{ movieID: string }>()

  return (
    <>
      <Link to="/" className="back-link">
        ← All movies
      </Link>
      <h1 className="page-title">Select seats</h1>
      <p className="page-subtitle">
        Seat selection for {movieID} is coming in the next step.
      </p>
    </>
  )
}
