import { idsForCollection } from "@/demo/params";
import { ProductionDetailView } from "./view";

// Server component: generateStaticParams cannot live in a "use client" file,
// which is why the client component moved to view.tsx.
export function generateStaticParams() {
  return idsForCollection("productions").map((id) => ({ id }));
}

export default function ProductionDetailPage() {
  return <ProductionDetailView />;
}
