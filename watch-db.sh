#!/bin/bash

# Watch database stats in real-time
while true; do
    clear
    echo "=== Database Stats - $(date) ==="
    echo ""
    
    docker exec vigil-postgres psql -U vigil -d vigil -t -c "
        SELECT 
            'Users: ' || COUNT(*)::text || 
            ' | Emails: ' || (SELECT COUNT(*) FROM emails)::text ||
            ' | Links: ' || (SELECT COUNT(*) FROM user_emails)::text
        FROM users;
    " 2>/dev/null | tr -d ' '
    
    echo ""
    echo "Top 10 users by email count:"
    docker exec vigil-postgres psql -U vigil -d vigil -c "
        SELECT 
            u.email,
            COUNT(ue.email_id) as emails,
            u.last_email_received
        FROM users u
        LEFT JOIN user_emails ue ON u.id = ue.user_id
        GROUP BY u.id, u.email, u.last_email_received
        ORDER BY emails DESC
        LIMIT 10;
    " 2>/dev/null
    
    echo ""
    echo "Press Ctrl+C to stop"
    sleep 5
done

