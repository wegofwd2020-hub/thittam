import { DocumentDetailView } from "./view";

// This route is not part of the demo slice, but `output: "export"` renders
// every route in the app, not just the reachable ones — so it still needs
// generateStaticParams or the export build fails outright.
//
// It returns a sentinel rather than []. Next 16 reports an empty array as
// "missing generateStaticParams()" and refuses to build, so one throwaway
// page is the price of exporting at all. Nothing links to it: the demo
// sidebar hides this whole section.
//
// dynamicParams stays at its default of true, so a normal server build
// still generates these on demand.
export function generateStaticParams() {
  return [{ id: "not-in-demo" }];
}

export default function DocumentDetailPage() {
  return <DocumentDetailView />;
}
