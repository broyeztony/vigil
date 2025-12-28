#!/bin/bash

echo "=== Vigil Database Inspector ==="
echo ""

echo "Table sizes:"
docker exec vigil-postgres psql -U vigil -d vigil -c "
SELECT 
    tablename,
    pg_size_pretty(pg_total_relation_size('public.'||tablename)) AS size
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY pg_total_relation_size('public.'||tablename) DESC;
"

echo ""
echo "Record counts:"
docker exec vigil-postgres psql -U vigil -d vigil -c "
SELECT 'tenants' as table_name, COUNT(*) as count FROM tenant
UNION ALL
SELECT 'users', COUNT(*) FROM users
UNION ALL
SELECT 'emails', COUNT(*) FROM emails
UNION ALL
SELECT 'user_emails', COUNT(*) FROM user_emails;
"

echo ""
echo "Recent emails (last 10):"
docker exec vigil-postgres psql -U vigil -d vigil -c "
SELECT 
    e.id,
    LEFT(e.fingerprint, 16) as fingerprint,
    e.received_at,
    COUNT(ue.user_id) as user_count
FROM emails e
LEFT JOIN user_emails ue ON e.id = ue.email_id
GROUP BY e.id, e.fingerprint, e.received_at
ORDER BY e.received_at DESC
LIMIT 10;
"

echo ""
echo "Top 20 users by email count:"
docker exec vigil-postgres psql -U vigil -d vigil -c "
SELECT 
    u.email,
    COUNT(ue.email_id) as email_count,
    u.last_email_received
FROM users u
LEFT JOIN user_emails ue ON u.id = ue.user_id
GROUP BY u.id, u.email, u.last_email_received
ORDER BY email_count DESC
LIMIT 20;
"

echo ""
echo "Recent activity (last 5 minutes):"
docker exec vigil-postgres psql -U vigil -d vigil -c "
SELECT 
    email,
    last_email_check,
    last_email_received,
    CASE 
        WHEN last_email_check IS NULL THEN 'Never'
        WHEN last_email_check > NOW() - INTERVAL '1 minute' THEN 'Active'
        WHEN last_email_check > NOW() - INTERVAL '5 minutes' THEN 'Recent'
        ELSE 'Stale'
    END as status
FROM users
WHERE last_email_check > NOW() - INTERVAL '5 minutes'
ORDER BY last_email_check DESC
LIMIT 20;
"
