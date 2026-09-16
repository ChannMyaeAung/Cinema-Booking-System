import { Link } from "react-router-dom";
import { useMovies } from "../api/queries";
import type { Movie } from "../api/api";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Spotlight } from "@/components/ui/spotlight";
import { Armchair } from "lucide-react";

function formatPrice(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

function MovieCard({ movie }: { movie: Movie }) {
  return (
    <Link to={`/movie/${encodeURIComponent(movie.id)}`} className="group block">
      <Card className="h-full pt-0 max-h-125 overflow-hidden transition-all duration-300 group-hover:-translate-y-1 group-hover:ring-amber-400/40 group-hover:shadow-[0_20px_60px_-20px] group-hover:shadow-amber-500/30">
        <div className="relative overflow-hidden ">
          <Spotlight className="left-0 top-0 h-[200%] w-[120%]" />
          {/* Poster */}
          <img
            alt={movie.title}
            src="/inception.jpg"
            className="h-full w-full object-cover"
          />
          <div className="grid aspect-2/3 place-items-center text-6xl font-extrabold tracking-tighter text-white/90 drop-shadow-[0_4px_24px_rgba(0,0,0,0.5)]">
            {movie.title.charAt(0).toUpperCase()}
          </div>
          <div className="pointer-events-none absolute inset-x-0 bottom-0 h-1/2 bg-linear-to-t from-black/70 to-transparent" />
        </div>

        <CardHeader className="pb-2">
          <CardTitle className="text-base leading-snug">
            {movie.title}
          </CardTitle>
          <CardDescription className="flex items-center gap-1.5">
            <Armchair className="size-3.5" />
            {movie.rows} rows · {movie.seats_per_row} seats per row
          </CardDescription>
          <CardDescription>20 Sep 2026</CardDescription>
        </CardHeader>

        <CardContent className="flex items-center justify-between pt-2">
          <Badge variant="secondary" className="text-xs">
            From {formatPrice(movie.price_cents)}
          </Badge>
          <span className="text-sm font-medium text-amber-400 transition-colors group-hover:text-amber-300">
            Book seats →
          </span>
        </CardContent>
      </Card>
    </Link>
  );
}

function SkeletonCard() {
  return (
    <Card className="overflow-hidden">
      <Skeleton className="aspect-2/3 w-full rounded-none" />
      <CardHeader className="pb-3">
        <Skeleton className="h-4 w-3/4" />
        <Skeleton className="h-3 w-1/2" />
      </CardHeader>
      <CardContent>
        <Skeleton className="h-8 w-full" />
      </CardContent>
    </Card>
  );
}

export default function Home() {
  const { data: movies, isPending, isError, error } = useMovies();

  return (
    <section>
      <div className="mb-8 flex flex-col items-start">
        <h1 className="font-heading text-3xl font-bold tracking-tight">
          Now showing
        </h1>
        <p className="mt-1 text-muted-foreground">
          Pick a film to choose your seats.
        </p>
      </div>

      {isPending && (
        <div
          className="grid grid-cols-2 gap-5 sm:grid-cols-3 lg:grid-cols-4"
          aria-hidden="true"
        >
          {Array.from({ length: 4 }).map((_, i) => (
            <SkeletonCard key={i} />
          ))}
        </div>
      )}

      {isError && (
        <div
          role="alert"
          className="rounded-xl border border-destructive/40 bg-destructive/10 p-4 text-sm text-destructive"
        >
          Could not load the movie list:{" "}
          {error instanceof Error ? error.message : "unknown error"}
        </div>
      )}

      {movies && (
        <div className="grid grid-cols-2 gap-5 sm:grid-cols-3 lg:grid-cols-4">
          {movies.map((movie) => (
            <MovieCard key={movie.id} movie={movie} />
          ))}
        </div>
      )}
    </section>
  );
}
