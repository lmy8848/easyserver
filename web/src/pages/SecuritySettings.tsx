import { Navigate } from 'react-router-dom';

export default function SecuritySettings() {
  return <Navigate to="/settings?tab=auth" replace />;
}

