// Owned dynamic SQL location fixture for dynamic-sql-r1 kind CSHARP_COMMAND (S05-A21).
// Expected facts and non-facts are registered in expected.json.
using System.Data;
using System.Data.Odbc;
using System.Data.OleDb;
using System.Data.SqlClient;

namespace Fixtures.DynamicSql
{
    internal static class Commands
    {
        internal static string BuildSql(string table)
        {
            return "SELECT * FROM " + table;
        }

        internal static void Run(SqlConnection conn, OleDbConnection oleConn, OdbcConnection odbcConn, string table, int id)
        {
            string sql = "SELECT 1 AS a";
            var c1 = new SqlCommand("SELECT 2 AS a", conn);
            var c2 = new SqlCommand(@"SELECT 3 AS a", conn);
            var c3 = new SqlCommand("""SELECT 4 AS a""", conn);
            var c4 = new System.Data.SqlClient.SqlCommand(sql, conn);
            var c5 = new OleDbCommand("SELECT * FROM " + table, oleConn);
            var c6 = new OdbcCommand($"SELECT * FROM {table} WHERE id = {id}", odbcConn);
            var c7 = new global::System.Data.SqlClient.SqlCommand("SELECT 5 AS a", conn);
            var a1 = new SqlDataAdapter(BuildSql(table), conn);
            var a2 = new OleDbDataAdapter(sql, oleConn);
            var a3 = new OdbcDataAdapter("SELECT 6 AS a", odbcConn);
            var c8 = new SqlCommand { Connection = conn, CommandText = "SELECT 7 AS a" };
            c1.CommandText = sql + " WHERE 1 = 1";
            var sp = new SqlCommand();
            sp.Connection = conn;
            sp.CommandType = CommandType.StoredProcedure;
            sp.CommandText = "dbo.GetOrders";
        }
    }
}
