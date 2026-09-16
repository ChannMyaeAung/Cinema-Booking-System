import { useCallback, useMemo, type ReactNode } from "react";
import { toast } from "sonner";
import { Toaster } from "@/components/ui/sonner";
import {
  ToastContext,
  type ToastTone,
} from "./toast-context";

export function ToastProvider({ children }: { children: ReactNode }) {
  const push = useCallback((message: string, tone: ToastTone = "info") => {
    if (tone === "success") {
      toast.success(message);
    } else if (tone === "error") {
      toast.error(message);
    } else {
      toast(message);
    }
  }, []);

  const value = useMemo(() => ({ push }), [push]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      <Toaster position="top-right" richColors closeButton />
    </ToastContext.Provider>
  );
}
