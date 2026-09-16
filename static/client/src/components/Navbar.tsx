import { Show, SignInButton, SignUpButton, UserButton } from "@clerk/react";
import { Link } from "react-router-dom";
import { Clapperboard } from "lucide-react";
import { Button } from "./ui/moving-border";

const Navbar = () => {
  return (
    <div className="mx-auto flex w-full max-w-6xl items-center justify-between py-3.5">
      <Link
        to="/"
        className="flex items-center gap-2.5 text-base font-bold tracking-tight"
      >
        <span className="grid size-8 place-items-center rounded-lg bg-linear-to-br from-amber-400 to-orange-600 text-primary-foreground shadow-[0_0_20px_-4px] shadow-amber-500/60">
          <Clapperboard className="size-4.5" />
        </span>
        CineBook
      </Link>

      <nav className="flex items-center gap-2">
        <Show when="signed-out">
          <Button variant="outline" size="sm" asChild>
            <SignInButton />
          </Button>
          <Button size="sm" asChild>
            <SignUpButton />
          </Button>
        </Show>
        <Show when="signed-in">
          <UserButton
            appearance={{
              elements: {
                avatarBox: "size-8 rounded-full",
              },
            }}
          />
        </Show>
      </nav>
    </div>
  );
};
export default Navbar;
