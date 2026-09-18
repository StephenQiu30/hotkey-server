import { Skeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <main
      className="bg-background min-h-screen px-5 py-20 sm:px-8 xl:px-0"
      aria-label="页面加载中"
      aria-busy="true"
    >
      <div className="mx-auto max-w-7xl">
        <div className="mx-auto flex max-w-3xl flex-col items-center">
          <Skeleton className="h-5 w-40 rounded-full" />
          <Skeleton className="mt-8 h-14 w-full max-w-2xl rounded-xl" />
          <Skeleton className="mt-4 h-7 w-full max-w-xl rounded-lg" />
        </div>
        <div className="mt-16 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:gap-6">
          {Array.from({ length: 3 }, (_, index) => (
            <Skeleton key={index} className="h-64 rounded-xl" />
          ))}
        </div>
      </div>
    </main>
  );
}
