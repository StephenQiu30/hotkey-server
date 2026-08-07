import { NotificationInbox } from "@/components/notifications/NotificationInbox";
import { ReportSubscriptions } from "@/components/notifications/ReportSubscriptions";

export default function NotificationsPage() {
  return (
    <div className="app-page space-y-12">
      <NotificationInbox />
      <ReportSubscriptions />
    </div>
  );
}
